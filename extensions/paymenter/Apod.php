<?php

namespace Paymenter\Extensions\Servers\Apod;

use App\Classes\Extension\Server;
use App\Models\Service;
use Exception;
use Illuminate\Support\Facades\Http;

class Apod extends Server
{
    private function request(string $endpoint, string $method = 'get', array $data = [])
    {
        $host = rtrim($this->config('host'), '/');
        $url = $host . '/api/v1' . $endpoint;

        $response = Http::withHeaders([
            'Authorization' => 'Bearer ' . $this->config('api_key'),
            'Content-Type' => 'application/json',
            'Accept' => 'application/json',
        ])->$method($url, $data);

        $body = $response->json();

        if (!$response->successful() || (isset($body['ok']) && !$body['ok'])) {
            $error = $body['error'] ?? 'Unknown error';
            throw new Exception('Apod API error: ' . $error);
        }

        return $body['data'] ?? $body;
    }

    public function getConfig($values = []): array
    {
        return [
            [
                'name' => 'host',
                'label' => 'Apod Server URL',
                'type' => 'text',
                'required' => true,
                'validation' => 'url',
                'description' => 'e.g., http://172.18.0.1:8443',
            ],
            [
                'name' => 'api_key',
                'label' => 'Admin API Key',
                'type' => 'password',
                'required' => true,
                'encrypted' => true,
                'description' => 'Admin API key from apod user create',
            ],
        ];
    }

    public function getProductConfig($values = []): array
    {
        // Fallback list — API fetch below overrides this with the real list
        $drivers = [
            ['label' => 'PHP + Nginx + MySQL', 'value' => 'php'],
            ['label' => 'Laravel', 'value' => 'laravel'],
            ['label' => 'WordPress', 'value' => 'wordpress'],
            ['label' => 'Node.js', 'value' => 'node'],
            ['label' => 'Static', 'value' => 'static'],
            ['label' => 'Odoo ERP', 'value' => 'odoo'],
            ['label' => 'Supabase', 'value' => 'supabase'],
        ];

        try {
            $result = $this->request('/drivers');
            if (is_array($result)) {
                $drivers = [];
                foreach ($result as $driver) {
                    $drivers[] = ['label' => $driver['description'] ?? $driver['name'], 'value' => $driver['name']];
                }
            }
        } catch (Exception $e) {}

        return [
            [
                'name' => 'driver',
                'label' => 'Site Driver',
                'type' => 'select',
                'required' => true,
                'options' => $drivers,
            ],
            [
                'name' => 'ram',
                'label' => 'RAM Limit',
                'type' => 'text',
                'required' => true,
                'default' => '256M',
            ],
            [
                'name' => 'cpu',
                'label' => 'CPU Limit',
                'type' => 'text',
                'required' => true,
                'default' => '1',
            ],
            [
                'name' => 'storage',
                'label' => 'Storage Limit',
                'type' => 'text',
                'required' => false,
                'default' => '5G',
            ],
        ];
    }

    public function getCheckoutConfig($product, $values = [], $settings = []): array
    {
        return [
            [
                'name' => 'domain',
                'label' => 'Domain',
                'type' => 'text',
                'required' => true,
                'description' => 'Your site domain (e.g., mysite.com)',
            ],
        ];
    }

    public function createServer(Service $service, $settings, $properties)
    {
        $domain = $properties['domain'] ?? null;
        if (!$domain) {
            throw new Exception('Domain is required');
        }

        // Create the site directly (admin key has full access)
        $this->request('/sites', 'post', [
            'domain' => $domain,
            'driver' => $settings['driver'] ?? 'php',
            'ram' => $settings['ram'] ?? '256M',
            'cpu' => $settings['cpu'] ?? '1',
            'storage' => $settings['storage'] ?? '0',
        ]);

        return true;
    }

    public function suspendServer(Service $service, $settings, $properties)
    {
        $domain = $properties['domain'] ?? null;
        if (!$domain) throw new Exception('Service has not been created');

        $this->request('/sites/' . $domain . '/stop', 'post');
        return true;
    }

    public function unsuspendServer(Service $service, $settings, $properties)
    {
        $domain = $properties['domain'] ?? null;
        if (!$domain) throw new Exception('Service has not been created');

        $this->request('/sites/' . $domain . '/start', 'post');
        return true;
    }

    public function terminateServer(Service $service, $settings, $properties)
    {
        $domain = $properties['domain'] ?? null;
        if (!$domain) throw new Exception('Service has not been created');

        try {
            $this->request('/sites/' . $domain, 'delete');
        } catch (Exception $e) {
            if (!str_contains($e->getMessage(), 'not found')) throw $e;
        }

        return true;
    }

    public function getActions(Service $service, $settings, $properties): array
    {
        $domain = $properties['domain'] ?? null;
        if (!$domain) return [];

        return [
            [
                'label' => 'Visit Site',
                'type' => 'button',
                'function' => 'visitSite',
            ],
        ];
    }

    public function visitSite(Service $service, $settings, $properties): string
    {
        return 'https://' . ($properties['domain'] ?? '');
    }

    public function testConfig(): bool
    {
        $this->request('/version');
        return true;
    }
}
