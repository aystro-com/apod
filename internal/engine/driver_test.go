package engine

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDriver(t *testing.T) {
	dir := t.TempDir()

	yaml := `
name: static
version: "1.0"
description: Static HTML site

parameters: {}

services:
  app:
    image: "nginx:alpine"
    volumes:
      - "${site_root}:/usr/share/nginx/html:ro"
    ports:
      - "80"

healthcheck:
  url: "http://localhost:80"
  interval: 10s
  timeout: 5s
  retries: 3
`
	os.WriteFile(filepath.Join(dir, "static.yaml"), []byte(yaml), 0644)

	loader := NewDriverLoader(dir)
	driver, err := loader.Load("static")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if driver.Name != "static" {
		t.Errorf("got name %q, want static", driver.Name)
	}
	if _, ok := driver.Services["app"]; !ok {
		t.Error("expected app service")
	}
	if driver.Services["app"].Image != "nginx:alpine" {
		t.Errorf("got image %q, want nginx:alpine", driver.Services["app"].Image)
	}
}

func TestLoadDriverNotFound(t *testing.T) {
	dir := t.TempDir()
	loader := NewDriverLoader(dir)

	_, err := loader.Load("nope")
	if err == nil {
		t.Fatal("expected error for missing driver")
	}
}

func TestListDrivers(t *testing.T) {
	dir := t.TempDir()

	os.WriteFile(filepath.Join(dir, "static.yaml"), []byte(`
name: static
version: "1.0"
description: Static HTML site
services:
  app:
    image: "nginx:alpine"
`), 0644)
	os.WriteFile(filepath.Join(dir, "wordpress.yaml"), []byte(`
name: wordpress
version: "1.0"
description: WordPress
services:
  app:
    image: "wordpress:latest"
`), 0644)

	loader := NewDriverLoader(dir)
	drivers, err := loader.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(drivers) != 2 {
		t.Errorf("got %d drivers, want 2", len(drivers))
	}
}

func TestExpandVariables(t *testing.T) {
	vars := map[string]string{
		"site_root": "/data/sites/example.com",
		"data_root": "/data/sites/example.com/data",
	}

	result := expandVariables("${site_root}:/usr/share/nginx/html:ro", vars)
	expected := "/data/sites/example.com:/usr/share/nginx/html:ro"
	if result != expected {
		t.Errorf("got %q, want %q", result, expected)
	}
}
