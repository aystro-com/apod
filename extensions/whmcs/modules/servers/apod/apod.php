<?php
/**
 * Apod WHMCS Provisioning Module
 *
 * Provisions web hosting sites via the apod REST API.
 * Install: copy the `apod` folder to /modules/servers/ in your WHMCS installation.
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
    ];
}

function apod_ConfigOptions()
{
    return [
        'Driver' => [
            'Type' => 'dropdown',
            'Options' => 'php,laravel,wordpress,node,static,odoo,unifi,paymenter',
            'Description' => 'Application driver',
            'Default' => 'php',
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

function apod_TestConnection(array $params)
{
    $response = apod_request($params, '/version', 'GET');

    if ($response['error']) {
        return ['success' => false, 'error' => $response['error']];
    }

    return ['success' => true, 'error' => ''];
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
