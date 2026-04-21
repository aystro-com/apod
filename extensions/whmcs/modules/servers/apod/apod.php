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

/**
 * Get the domain for a service — checks custom field "Domain" first, then falls back to $params['domain']
 */
function apod_getDomain(array $params): string
{
    // Check WHMCS domain field first (set by CreateAccount on first provision)
    if (!empty($params['domain'])) {
        return $params['domain'];
    }

    // Check custom fields passed by WHMCS
    $customfields = $params['customfields'] ?? [];
    if (!empty($customfields['Domain'])) {
        return $customfields['Domain'];
    }

    // Last resort: look up custom field directly from DB
    if (!empty($params['serviceid'])) {
        try {
            $val = \WHMCS\Database\Capsule::table('tblcustomfieldsvalues')
                ->join('tblcustomfields', 'tblcustomfields.id', '=', 'tblcustomfieldsvalues.fieldid')
                ->where('tblcustomfields.fieldname', 'Domain')
                ->where('tblcustomfieldsvalues.relid', $params['serviceid'])
                ->value('tblcustomfieldsvalues.value');
            if (!empty($val)) {
                return $val;
            }
        } catch (\Exception $e) {}
    }

    return '';
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
    $domain = apod_getDomain($params);
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

    // Store domain in WHMCS service record
    try {
        \WHMCS\Database\Capsule::table('tblhosting')
            ->where('id', $params['serviceid'])
            ->update(['domain' => $domain]);
    } catch (\Exception $e) {}

    return 'success';
}

function apod_SuspendAccount(array $params)
{
    $domain = apod_getDomain($params);
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
    $domain = apod_getDomain($params);
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
    $domain = apod_getDomain($params);
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
    $domain = apod_getDomain($params);
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

    // Container shell access (web terminal embedded in WHMCS)
    if ($shellEnabled && $status === 'running') {
        // Get a terminal token from apod
        $tokenResp = apod_request($params, '/sites/' . $domain . '/terminal', 'POST');
        $termToken = $tokenResp['data']['token'] ?? '';

        if ($termToken) {
            $execUrl = '/modules/servers/apod/terminal_proxy.php';
            $serviceId = $params['serviceid'] ?? 0;

            $html .= '<hr><h4>Container Shell</h4>';
            $html .= '<p class="text-muted">Commands run inside your isolated container. Token expires in 5 minutes — refresh for a new one.</p>';
            $html .= '<div style="background:#1e1e1e;border-radius:6px;padding:15px;font-family:monospace;font-size:13px;color:#0f0;min-height:300px;max-height:500px;overflow-y:auto;text-align:left" id="apod-terminal-output">';
            $html .= '<div>Welcome to ' . htmlspecialchars($domain) . '</div>';
            $html .= '<div>Type a command and press Enter.</div><div>&nbsp;</div>';
            $html .= '</div>';
            $html .= '<div style="margin-top:8px;display:flex;gap:8px">';
            $html .= '<span style="font-family:monospace;color:#666;line-height:34px">$</span>';
            $html .= '<input type="text" id="apod-terminal-input" class="form-control" placeholder="ls -la" style="font-family:monospace;flex:1" autocomplete="off">';
            $html .= '<button class="btn btn-success" id="apod-terminal-run">Run</button>';
            $html .= '</div>';
            $html .= '<script>
(function() {
    var token = "' . $termToken . '";
    var execUrl = "' . $execUrl . '";
    var serviceId = ' . intval($serviceId) . ';
    var output = document.getElementById("apod-terminal-output");
    var input = document.getElementById("apod-terminal-input");
    var btn = document.getElementById("apod-terminal-run");

    function run() {
        var cmd = input.value.trim();
        if (!cmd) return;
        output.innerHTML += "<div style=\"color:#fff\">$ " + cmd.replace(/</g,"&lt;") + "</div>";
        input.value = "";
        btn.disabled = true;
        btn.textContent = "...";

        fetch(execUrl, {
            method: "POST",
            headers: {"Content-Type": "application/json"},
            body: JSON.stringify({token: token, command: cmd, service_id: serviceId})
        })
        .then(function(r) { return r.json(); })
        .then(function(data) {
            if (data.ok && data.data && data.data.output) {
                output.innerHTML += "<pre style=\"color:#ccc;margin:0;white-space:pre-wrap\">" + data.data.output.replace(/</g,"&lt;") + "</pre>";
            } else {
                output.innerHTML += "<div style=\"color:#f55\">" + (data.error || "Error") + "</div>";
            }
            output.scrollTop = output.scrollHeight;
            btn.disabled = false;
            btn.textContent = "Run";
        })
        .catch(function(e) {
            output.innerHTML += "<div style=\"color:#f55\">Connection error</div>";
            btn.disabled = false;
            btn.textContent = "Run";
        });
    }

    btn.addEventListener("click", run);
    input.addEventListener("keydown", function(e) { if (e.key === "Enter") run(); });
    input.focus();
})();
</script>';
        }
    } else if ($shellEnabled) {
        $html .= '<hr><h4>Container Shell</h4>';
        $html .= '<p class="text-muted">Terminal is available when the site is running.</p>';
    }

    // Backups
    if ($backupsEnabled) {
        $html .= '<hr><h4>Backups</h4>';

        $backupResp = apod_request($params, '/sites/' . $domain . '/backups', 'GET');
        $backups = $backupResp['data'] ?? [];

        if (is_array($backups) && count($backups) > 0) {
            $html .= '<table class="table table-condensed table-striped">';
            $sid = $params['serviceid'] ?? '';
            $html .= '<thead><tr><th>ID</th><th>Storage</th><th>Size</th><th>Date</th><th>Actions</th></tr></thead><tbody>';
            foreach ($backups as $b) {
                $bid = $b['id'] ?? '';
                $size = isset($b['size_bytes']) ? round($b['size_bytes'] / 1024 / 1024, 1) . ' MB' : '-';
                $html .= '<tr>';
                $html .= '<td>' . $bid . '</td>';
                $html .= '<td>' . ($b['storage_name'] ?? 'local') . '</td>';
                $html .= '<td>' . $size . '</td>';
                $html .= '<td>' . ($b['created_at'] ?? '-') . '</td>';
                $html .= '<td>';
                $html .= '<a href="clientarea.php?action=productdetails&id=' . $sid . '&modop=custom&a=downloadBackup&backup_id=' . $bid . '" class="btn btn-xs btn-default">Download</a> ';
                $html .= '<a href="#" onclick="if(confirm(\'Are you sure you want to restore this backup? This will overwrite your current site data.\')){window.location=\'clientarea.php?action=productdetails&id=' . $sid . '&modop=custom&a=restoreBackupById&backup_id=' . $bid . '\';}return false;" class="btn btn-xs btn-warning">Restore</a>';
                $html .= '</td>';
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

    // Backups
    if (!empty($params['configoption6'])) {
        $buttons['Create Backup'] = 'createBackup';
    }

    return $buttons;
}

function apod_openTerminal(array $params)
{
    return 'Use the terminal in the service details page';
}

/**
 * AJAX proxy for terminal exec — called by the embedded terminal JS
 * This avoids CORS issues since the request goes WHMCS → apod, not browser → apod
 */
function apod_terminalProxy()
{
    if (!defined("WHMCS")) return;
    if ($_SERVER['REQUEST_METHOD'] !== 'POST') return;

    $input = json_decode(file_get_contents('php://input'), true);
    $token = $input['token'] ?? '';
    $command = $input['command'] ?? '';
    $serviceId = $input['service_id'] ?? 0;

    if (empty($token) || empty($command) || empty($serviceId)) {
        header('Content-Type: application/json');
        echo json_encode(['ok' => false, 'error' => 'Missing parameters']);
        return;
    }

    // Verify the user owns this service
    $service = \WHMCS\Database\Capsule::table('tblhosting')
        ->where('id', $serviceId)
        ->first();

    if (!$service) {
        header('Content-Type: application/json');
        echo json_encode(['ok' => false, 'error' => 'Service not found']);
        return;
    }

    // Get server details
    $server = \WHMCS\Database\Capsule::table('tblservers')
        ->where('type', 'apod')
        ->first();

    if (!$server) {
        header('Content-Type: application/json');
        echo json_encode(['ok' => false, 'error' => 'Server not configured']);
        return;
    }

    $host = $server->hostname ?: $server->ipaddress;
    $port = $server->port ?: '8443';
    $url = 'http://' . $host . ':' . $port . '/api/v1/terminal/exec';

    $ch = curl_init();
    curl_setopt($ch, CURLOPT_URL, $url);
    curl_setopt($ch, CURLOPT_POST, true);
    curl_setopt($ch, CURLOPT_RETURNTRANSFER, true);
    curl_setopt($ch, CURLOPT_TIMEOUT, 30);
    curl_setopt($ch, CURLOPT_HTTPHEADER, ['Content-Type: application/json']);
    curl_setopt($ch, CURLOPT_POSTFIELDS, json_encode(['token' => $token, 'command' => $command]));
    $result = curl_exec($ch);
    curl_close($ch);

    header('Content-Type: application/json');
    echo $result ?: json_encode(['ok' => false, 'error' => 'Connection failed']);
}

function apod_restart(array $params)
{
    $domain = apod_getDomain($params);
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
    $domain = apod_getDomain($params);
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
    $domain = apod_getDomain($params);
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

function apod_restoreBackupById(array $params)
{
    $domain = apod_getDomain($params);
    if (empty($domain)) {
        return 'Domain is required';
    }
    if (empty($params['configoption6'])) {
        return 'Backups are not enabled for this product';
    }

    $backupId = intval($_GET['backup_id'] ?? 0);
    if ($backupId < 1) {
        return 'Invalid backup ID';
    }

    $response = apod_request($params, '/sites/' . $domain . '/backups/restore', 'POST', [
        'backup_id' => $backupId,
    ]);
    if ($response['error']) {
        return $response['error'];
    }

    return 'success';
}

function apod_downloadBackup(array $params)
{
    $domain = apod_getDomain($params);
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
        return 'No backups available';
    }

    $latestId = end($backups)['id'] ?? null;
    if (!$latestId) {
        return 'No backup found';
    }

    // Download via the API
    $response = apod_request($params, '/sites/' . $domain . '/backups/download', 'POST', [
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
    $domain = apod_getDomain($params);
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
    $domain = apod_getDomain($params);
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
    $host = rtrim($params['serverhostname'] ?: ($params['serverip'] ?? ''), '/');
    $port = $params['serverport'] ?? '';
    $scheme = !empty($params['serversecure']) ? 'https' : 'http';

    // Fetch from DB if missing
    if (empty($host) || empty($port)) {
        try {
            $srv = \WHMCS\Database\Capsule::table('tblservers')->where('type', 'apod')->first();
            if ($srv) {
                if (empty($host)) $host = $srv->hostname ?: $srv->ipaddress;
                if (empty($port)) $port = $srv->port;
            }
        } catch (\Exception $e) {}
    }
    if (empty($port)) $port = '8443';
    // Always fetch API key from DB to avoid encryption issues
    $apiKey = $params['serverpassword'] ?? '';
    if (empty($apiKey) || !str_starts_with($apiKey, 'apod_')) {
        try {
            $serverId = $params['serverid'] ?? null;
            if ($serverId) {
                $apiKey = \WHMCS\Database\Capsule::table('tblservers')->where('id', $serverId)->value('password');
            } else {
                $apiKey = \WHMCS\Database\Capsule::table('tblservers')->where('type', 'apod')->value('password');
            }
        } catch (\Exception $e) {}
        // If still encrypted, try decrypt
        if (!empty($apiKey) && !str_starts_with($apiKey, 'apod_')) {
            try { $apiKey = decrypt($apiKey); } catch (\Exception $e) {}
        }
    }

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
