<?php
/**
 * Terminal proxy — routes terminal commands from the browser to apod's API.
 * This avoids CORS issues since browser → WHMCS → apod (server-side).
 */

define("WHMCS", true);
require __DIR__ . '/../../../init.php';

use WHMCS\Database\Capsule;

header('Content-Type: application/json');

if ($_SERVER['REQUEST_METHOD'] !== 'POST') {
    echo json_encode(['ok' => false, 'error' => 'POST required']);
    exit;
}

$input = json_decode(file_get_contents('php://input'), true);
$token = $input['token'] ?? '';
$command = $input['command'] ?? '';
$serviceId = intval($input['service_id'] ?? 0);

if (empty($token) || empty($command) || $serviceId < 1) {
    echo json_encode(['ok' => false, 'error' => 'Missing parameters']);
    exit;
}

// Verify the logged-in user owns this service
session_start();
$userId = $_SESSION['uid'] ?? 0;
if ($userId < 1) {
    echo json_encode(['ok' => false, 'error' => 'Not authenticated']);
    exit;
}

$service = Capsule::table('tblhosting')
    ->where('id', $serviceId)
    ->where('userid', $userId)
    ->first();

if (!$service) {
    echo json_encode(['ok' => false, 'error' => 'Access denied']);
    exit;
}

// Get apod server
$server = Capsule::table('tblservers')->where('type', 'apod')->first();
if (!$server) {
    echo json_encode(['ok' => false, 'error' => 'Server not configured']);
    exit;
}

$host = $server->hostname ?: $server->ipaddress;
$port = $server->port ?: '8443';
$url = 'http://' . $host . ':' . $port . '/api/v1/terminal/exec';

// Proxy the request
$ch = curl_init();
curl_setopt($ch, CURLOPT_URL, $url);
curl_setopt($ch, CURLOPT_POST, true);
curl_setopt($ch, CURLOPT_RETURNTRANSFER, true);
curl_setopt($ch, CURLOPT_TIMEOUT, 30);
curl_setopt($ch, CURLOPT_SSL_VERIFYHOST, 0);
curl_setopt($ch, CURLOPT_SSL_VERIFYPEER, 0);
curl_setopt($ch, CURLOPT_HTTPHEADER, ['Content-Type: application/json']);
curl_setopt($ch, CURLOPT_POSTFIELDS, json_encode([
    'token' => $token,
    'command' => $command,
]));

$result = curl_exec($ch);
$error = curl_error($ch);
curl_close($ch);

if ($error) {
    echo json_encode(['ok' => false, 'error' => 'Connection failed: ' . $error]);
    exit;
}

echo $result;
