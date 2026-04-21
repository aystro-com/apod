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
    ];
}

function apod_CreateAccount(array $params)
{
    $domain = $params['domain'];
    if (empty($domain)) {
        return 'Domain is required';
    }

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

    // Store SSH/SFTP credentials for the client
    // The site's Linux user is derived from the domain
    $sshUser = preg_replace('/[^a-z0-9\-]/', '-', strtolower($domain));
    $sshUser = substr(trim($sshUser, '-'), 0, 32);
    if (!preg_match('/^[a-z]/', $sshUser)) {
        $sshUser = 'u-' . $sshUser;
    }

    // Save username to service
    try {
        \WHMCS\Database\Capsule::table('tblhosting')
            ->where('id', $params['serviceid'])
            ->update(['username' => $sshUser]);
    } catch (\Exception $e) {
        // Non-critical
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
 * Client area actions — buttons shown to the customer
 */
function apod_ClientArea(array $params)
{
    $domain = $params['domain'];
    if (empty($domain)) {
        return '';
    }

    $serverHost = $params['serverhostname'] ?: $params['serverip'];

    // Get site status
    $response = apod_request($params, '/sites/' . $domain . '/monitor', 'GET');
    $stats = $response['data'] ?? [];

    $status = $stats['status'] ?? 'Unknown';
    $statusBadge = $status === 'running'
        ? '<span class="label label-success">Running</span>'
        : '<span class="label label-danger">' . ucfirst($status) . '</span>';

    $html = '<div class="row">';
    $html .= '<div class="col-sm-4"><strong>Status:</strong> ' . $statusBadge . '</div>';
    $html .= '<div class="col-sm-4"><strong>CPU:</strong> ' . round($stats['cpu_percent'] ?? 0, 1) . '%</div>';
    $html .= '<div class="col-sm-4"><strong>RAM:</strong> ' . round($stats['memory_mb'] ?? 0) . 'MB / ' . round($stats['memory_limit_mb'] ?? 0) . 'MB</div>';
    $html .= '</div>';

    // SSH/SFTP access info
    $html .= '<div class="row" style="margin-top:20px">';
    $html .= '<div class="col-sm-12"><h4>SFTP / SSH Access</h4></div>';
    $html .= '<div class="col-sm-4"><strong>Host:</strong> ' . htmlspecialchars($serverHost) . '</div>';
    $html .= '<div class="col-sm-4"><strong>Username:</strong> ' . htmlspecialchars($params['username'] ?? '-') . '</div>';
    $html .= '<div class="col-sm-4"><strong>Port:</strong> 22</div>';
    $html .= '</div>';
    $html .= '<p class="text-muted" style="margin-top:5px">Use your SFTP client (FileZilla, WinSCP) to upload files to your site.</p>';

    // Actions
    $html .= '<div class="row" style="margin-top:20px">';
    $html .= '<div class="col-sm-12">';
    $html .= '<a href="https://' . htmlspecialchars($domain) . '" target="_blank" class="btn btn-primary">Visit Site</a> ';
    $html .= '</div>';
    $html .= '</div>';

    return $html;
}

/**
 * Client area custom button actions
 */
function apod_ClientAreaCustomButtonArray()
{
    return [
        'Restart Site' => 'restart',
    ];
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

/**
 * Admin area custom actions
 */
function apod_AdminCustomButtonArray()
{
    return [
        'Restart Site' => 'restart',
    ];
}

/**
 * Admin service info panel — shows site details in admin area
 */
function apod_AdminServicesTabFields(array $params)
{
    $domain = $params['domain'];
    if (empty($domain)) {
        return [];
    }

    $response = apod_request($params, '/sites/' . $domain, 'GET');
    $site = $response['data'] ?? [];

    return [
        'Site URL' => '<a href="https://' . htmlspecialchars($domain) . '" target="_blank">https://' . htmlspecialchars($domain) . '</a>',
        'Driver' => $site['driver'] ?? '-',
        'Status' => $site['status'] ?? '-',
        'RAM' => $site['ram'] ?? '-',
        'CPU' => $site['cpu'] ?? '-',
        'Storage' => $site['storage'] ?? '-',
        'Created' => $site['created_at'] ?? '-',
    ];
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
