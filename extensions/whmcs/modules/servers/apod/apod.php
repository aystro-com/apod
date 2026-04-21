<?php
/**
 * Apod WHMCS Provisioning Module
 *
 * Provisions web hosting sites via the apod REST API.
 * Install: copy the `apod` folder to /modules/servers/ in your WHMCS installation.
 *
 * Server configuration in WHMCS:
 *   - Hostname: apod server IP or hostname
 *   - Port: 8443 (or your TCP listener port)
 *   - Password: admin API key (from `apod user create`)
 *
 * Product ConfigOptions:
 *   1. Driver (php, laravel, wordpress, node, static, odoo, unifi)
 *   2. RAM (256M, 512M, 1G, 2G)
 *   3. CPU (1, 2, 4)
 *   4. Storage (1G, 5G, 10G, 50G)
 *   5. SSH Access (yes/no) — gives customer shell access
 *   6. Backups (yes/no) — allows customer to create/restore backups
 *
 * @see https://github.com/aystro-com/apod
 */

if (!defined("WHMCS")) {
    die("This file cannot be accessed directly");
}

function apod_MetaData()
{
    return [
        'DisplayName' => 'Apod',
        'APIVersion' => '1.1',
        'RequiresServer' => true,
        'DefaultNonSSLPort' => '8443',
        'DefaultSSLPort' => '8443',
        'ServiceSingleSignOnLabel' => 'Visit Site',
    ];
}

function apod_ConfigOptions()
{
    return [
        'Driver' => [
            'Type' => 'text',
            'Size' => '20',
            'Default' => 'php',
            'Description' => 'Driver name (use Test Connection to see available drivers)',
        ],
        'RAM' => [
            'Type' => 'text',
            'Size' => '10',
            'Default' => '256M',
            'Description' => 'RAM limit (e.g., 256M, 512M, 1G)',
        ],
        'CPU' => [
            'Type' => 'text',
            'Size' => '5',
            'Default' => '1',
            'Description' => 'CPU cores',
        ],
        'Storage' => [
            'Type' => 'text',
            'Size' => '10',
            'Default' => '5G',
            'Description' => 'Disk quota (e.g., 1G, 5G, 10G)',
        ],
        'Shell Access' => [
            'Type' => 'yesno',
            'Description' => 'Allow web terminal access to the container (fully isolated)',
        ],
        'Backups' => [
            'Type' => 'yesno',
            'Description' => 'Allow customer to create and restore backups',
        ],
    ];
}

function apod_CreateAccount(array $params)
{
    $domain = $params['domain'];
    if (empty($domain)) {
        return 'Domain is required';
    }

    // Create the site
    $response = apod_request($params, '/sites', 'POST', [
        'domain' => $domain,
        'driver' => $params['configoption1'] ?: 'php',
        'ram' => $params['configoption2'] ?: '256M',
        'cpu' => $params['configoption3'] ?: '1',
        'storage' => $params['configoption4'] ?: '0',
    ]);

    if ($response['error']) {
        return $response['error'];
    }

    return 'success';
}

function apod_SuspendAccount(array $params)
{
    $domain = $params['domain'];
    if (empty($domain)) {
        return 'Domain is required';
    }

    $response = apod_request($params, '/sites/' . $domain . '/stop', 'POST');

    if ($response['error']) {
        return $response['error'];
    }

    return 'success';
}

function apod_UnsuspendAccount(array $params)
{
    $domain = $params['domain'];
    if (empty($domain)) {
        return 'Domain is required';
    }

    $response = apod_request($params, '/sites/' . $domain . '/start', 'POST');

    if ($response['error']) {
        return $response['error'];
    }

    return 'success';
}

function apod_TerminateAccount(array $params)
{
    $domain = $params['domain'];
    if (empty($domain)) {
        return 'Domain is required';
    }

    $response = apod_request($params, '/sites/' . $domain, 'DELETE');
    if ($response['error'] && !str_contains($response['error'], 'not found')) {
        return $response['error'];
    }

    return 'success';
}

/**
 * Client area — shows site info, SFTP details, backups
 */
function apod_ClientArea(array $params)
{
    $domain = $params['domain'];
    if (empty($domain)) {
        return '';
    }

    $serverHost = $params['serverhostname'] ?: $params['serverip'];
    $shellEnabled = !empty($params['configoption5']);
    $backupsEnabled = !empty($params['configoption6']);

    // Get site status
    $response = apod_request($params, '/sites/' . $domain . '/monitor', 'GET');
    $stats = $response['data'] ?? [];

    $status = $stats['status'] ?? 'Unknown';
    $statusBadge = $status === 'running'
        ? '<span class="label label-success">Running</span>'
        : '<span class="label label-danger">' . ucfirst($status) . '</span>';

    // Site overview
    $html = '<h4>Site Overview</h4>';
    $html .= '<div class="row">';
    $html .= '<div class="col-sm-3"><strong>Status:</strong> ' . $statusBadge . '</div>';
    $html .= '<div class="col-sm-3"><strong>CPU:</strong> ' . round($stats['cpu_percent'] ?? 0, 1) . '%</div>';
    $html .= '<div class="col-sm-3"><strong>RAM:</strong> ' . round($stats['memory_mb'] ?? 0) . 'MB / ' . round($stats['memory_limit_mb'] ?? 0) . 'MB</div>';
    $html .= '<div class="col-sm-3"><strong>Driver:</strong> ' . htmlspecialchars($params['configoption1'] ?? 'php') . '</div>';
    $html .= '</div>';

    // Container shell access (web terminal)
    if ($shellEnabled) {
        $html .= '<hr><h4>Container Shell</h4>';
        $html .= '<p>Access your site\'s container directly via web terminal. All commands run inside your isolated container — fully sandboxed from other sites.</p>';
    }

    // Backups
    if ($backupsEnabled) {
        $html .= '<hr><h4>Backups</h4>';

        $backupResp = apod_request($params, '/sites/' . $domain . '/backups', 'GET');
        $backups = $backupResp['data'] ?? [];

        if (is_array($backups) && count($backups) > 0) {
            $html .= '<table class="table table-condensed table-striped">';
            $html .= '<thead><tr><th>ID</th><th>Storage</th><th>Size</th><th>Date</th></tr></thead><tbody>';
            foreach ($backups as $b) {
                $size = isset($b['size_bytes']) ? round($b['size_bytes'] / 1024 / 1024, 1) . ' MB' : '-';
                $html .= '<tr>';
                $html .= '<td>' . ($b['id'] ?? '-') . '</td>';
                $html .= '<td>' . ($b['storage_name'] ?? 'local') . '</td>';
                $html .= '<td>' . $size . '</td>';
                $html .= '<td>' . ($b['created_at'] ?? '-') . '</td>';
                $html .= '</tr>';
            }
            $html .= '</tbody></table>';
        } else {
            $html .= '<p class="text-muted">No backups yet.</p>';
        }
    }

    // Actions
    $html .= '<hr><div class="row" style="margin-top:10px">';
    $html .= '<div class="col-sm-12">';
    $html .= '<a href="https://' . htmlspecialchars($domain) . '" target="_blank" class="btn btn-primary">Visit Site</a> ';
    $html .= '</div>';
    $html .= '</div>';

    return $html;
}

/**
 * Client area buttons — based on product config
 */
function apod_ClientAreaCustomButtonArray(array $params = [])
{
    $buttons = [
        'Restart Site' => 'restart',
    ];

    // Shell access
    if (!empty($params['configoption5'])) {
        $buttons['Open Terminal'] = 'openTerminal';
    }

    // Backups
    if (!empty($params['configoption6'])) {
        $buttons['Create Backup'] = 'createBackup';
        $buttons['Restore Latest Backup'] = 'restoreBackup';
    }

    return $buttons;
}

function apod_openTerminal(array $params)
{
    $domain = $params['domain'];
    if (empty($domain)) {
        return 'Domain is required';
    }
    if (empty($params['configoption5'])) {
        return 'Shell access is not enabled for this product';
    }

    // Execute a command in the container via the API
    // For web terminal, this would redirect to a websocket terminal
    // For now, return a URL that the admin/customer can use
    $serverHost = $params['serverhostname'] ?: $params['serverip'];
    $port = $params['serverport'] ?: '8443';
    $scheme = $params['serversecure'] ? 'https' : 'http';

    // The actual web terminal would be served by apod at this URL
    header('Location: ' . $scheme . '://' . $serverHost . ':' . $port . '/terminal/' . $domain);
    exit;
}

function apod_restart(array $params)
{
    $domain = $params['domain'];
    if (empty($domain)) {
        return 'Domain is required';
    }

    $response = apod_request($params, '/sites/' . $domain . '/restart', 'POST');
    if ($response['error']) {
        return $response['error'];
    }

    return 'success';
}

function apod_createBackup(array $params)
{
    $domain = $params['domain'];
    if (empty($domain)) {
        return 'Domain is required';
    }
    if (empty($params['configoption6'])) {
        return 'Backups are not enabled for this product';
    }

    $response = apod_request($params, '/sites/' . $domain . '/backups', 'POST', [
        'storage' => 'local',
    ]);
    if ($response['error']) {
        return $response['error'];
    }

    return 'success';
}

function apod_restoreBackup(array $params)
{
    $domain = $params['domain'];
    if (empty($domain)) {
        return 'Domain is required';
    }
    if (empty($params['configoption6'])) {
        return 'Backups are not enabled for this product';
    }

    // Get latest backup
    $listResp = apod_request($params, '/sites/' . $domain . '/backups', 'GET');
    $backups = $listResp['data'] ?? [];

    if (empty($backups)) {
        return 'No backups available to restore';
    }

    // Restore the most recent one
    $latestId = end($backups)['id'] ?? null;
    if (!$latestId) {
        return 'No backup ID found';
    }

    $response = apod_request($params, '/sites/' . $domain . '/backups/restore', 'POST', [
        'backup_id' => $latestId,
    ]);
    if ($response['error']) {
        return $response['error'];
    }

    return 'success';
}

/**
 * Admin area custom actions
 */
function apod_AdminCustomButtonArray()
{
    return [
        'Restart Site' => 'restart',
        'Create Backup' => 'createBackup',
    ];
}

/**
 * Admin service info panel
 */
function apod_AdminServicesTabFields(array $params)
{
    $domain = $params['domain'];
    if (empty($domain)) {
        return [];
    }

    $response = apod_request($params, '/sites/' . $domain, 'GET');
    $site = $response['data'] ?? [];

    $fields = [
        'Site URL' => '<a href="https://' . htmlspecialchars($domain) . '" target="_blank">https://' . htmlspecialchars($domain) . '</a>',
        'Driver' => $site['driver'] ?? '-',
        'Status' => $site['status'] ?? '-',
        'RAM' => $site['ram'] ?? '-',
        'CPU' => $site['cpu'] ?? '-',
        'Storage' => $site['storage'] ?? '-',
        'Owner' => $site['owner'] ?? '(admin)',
        'Created' => $site['created_at'] ?? '-',
    ];

    $fields['Shell Access'] = !empty($params['configoption5']) ? 'Enabled (container only)' : 'Disabled';
    $fields['Backups'] = !empty($params['configoption6']) ? 'Enabled' : 'Disabled';

    return $fields;
}

/**
 * SSO — redirect to site
 */
function apod_ServiceSingleSignOn(array $params)
{
    $domain = $params['domain'];
    return ['success' => true, 'redirectTo' => 'https://' . $domain];
}

function apod_TestConnection(array $params)
{
    $response = apod_request($params, '/version', 'GET');

    if ($response['error']) {
        return ['success' => false, 'error' => $response['error']];
    }

    $drivers = apod_request($params, '/drivers', 'GET');
    $driverList = '';
    if (!$drivers['error'] && is_array($drivers['data'])) {
        $names = array_map(function ($d) { return $d['name']; }, $drivers['data']);
        $driverList = ' | Drivers: ' . implode(', ', $names);
    }

    $version = $response['data']['version'] ?? 'unknown';
    return ['success' => true, 'error' => 'Connected to apod v' . $version . $driverList];
}

/**
 * Make an API request to the apod server.
 */
function apod_request(array $params, string $endpoint, string $method = 'GET', array $data = [])
{
    $host = rtrim($params['serverhostname'] ?: $params['serverip'], '/');
    $port = $params['serverport'] ?: '8443';
    $scheme = $params['serversecure'] ? 'https' : 'http';
    $apiKey = $params['serverpassword'];

    $url = $scheme . '://' . $host . ':' . $port . '/api/v1' . $endpoint;

    $ch = curl_init();
    curl_setopt($ch, CURLOPT_URL, $url);
    curl_setopt($ch, CURLOPT_RETURNTRANSFER, true);
    curl_setopt($ch, CURLOPT_TIMEOUT, 120);
    curl_setopt($ch, CURLOPT_SSL_VERIFYHOST, 0);
    curl_setopt($ch, CURLOPT_SSL_VERIFYPEER, 0);
    curl_setopt($ch, CURLOPT_HTTPHEADER, [
        'Authorization: Bearer ' . $apiKey,
        'Content-Type: application/json',
        'Accept: application/json',
    ]);

    if ($method === 'POST') {
        curl_setopt($ch, CURLOPT_POST, true);
        curl_setopt($ch, CURLOPT_POSTFIELDS, json_encode($data));
    } elseif ($method === 'DELETE') {
        curl_setopt($ch, CURLOPT_CUSTOMREQUEST, 'DELETE');
    }

    $result = curl_exec($ch);
    $httpCode = curl_getinfo($ch, CURLINFO_HTTP_CODE);
    $error = curl_error($ch);
    curl_close($ch);

    if ($error) {
        return ['error' => 'Connection failed: ' . $error, 'data' => null];
    }

    $body = json_decode($result, true);

    if ($httpCode >= 400 || (isset($body['ok']) && !$body['ok'])) {
        return ['error' => $body['error'] ?? 'HTTP ' . $httpCode, 'data' => null];
    }

    return ['error' => null, 'data' => $body['data'] ?? $body];
}
