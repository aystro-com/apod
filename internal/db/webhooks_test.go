package db

import "testing"

func TestCreateWebhook(t *testing.T) {
	d := openTestDB(t)
	err := d.CreateWebhook("example.com", "tok_abc123")
	if err != nil {
		t.Fatalf("CreateWebhook: %v", err)
	}
}

func TestGetWebhookByToken(t *testing.T) {
	d := openTestDB(t)
	d.CreateWebhook("example.com", "tok_abc123")
	wh, err := d.GetWebhookByToken("tok_abc123")
	if err != nil {
		t.Fatalf("GetWebhookByToken: %v", err)
	}
	if wh.SiteDomain != "example.com" {
		t.Errorf("got domain %q", wh.SiteDomain)
	}
}

func TestListWebhooks(t *testing.T) {
	d := openTestDB(t)
	d.CreateWebhook("example.com", "tok_1")
	d.CreateWebhook("example.com", "tok_2")
	whs, err := d.ListWebhooks("example.com")
	if err != nil {
		t.Fatalf("ListWebhooks: %v", err)
	}
	if len(whs) != 2 {
		t.Errorf("got %d, want 2", len(whs))
	}
}

func TestDeleteWebhook(t *testing.T) {
	d := openTestDB(t)
	d.CreateWebhook("example.com", "tok_1")
	err := d.DeleteWebhook("example.com")
	if err != nil {
		t.Fatalf("DeleteWebhook: %v", err)
	}
	whs, _ := d.ListWebhooks("example.com")
	if len(whs) != 0 {
		t.Errorf("got %d, want 0", len(whs))
	}
}
