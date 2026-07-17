package db

import "testing"

func TestCreateProxyRule(t *testing.T) {
	d := openTestDB(t)
	id, err := d.CreateProxyRule("example.com", "redirect", `{"from":"/old","to":"/new"}`)
	if err != nil {
		t.Fatalf("CreateProxyRule: %v", err)
	}
	if id == 0 {
		t.Error("expected ID")
	}
}

func TestListProxyRules(t *testing.T) {
	d := openTestDB(t)
	d.CreateProxyRule("example.com", "redirect", `{}`)
	d.CreateProxyRule("example.com", "header", `{}`)
	rules, err := d.ListProxyRules("example.com")
	if err != nil {
		t.Fatalf("ListProxyRules: %v", err)
	}
	if len(rules) != 2 {
		t.Errorf("got %d, want 2", len(rules))
	}
}

func TestDeleteProxyRuleForSite(t *testing.T) {
	d := openTestDB(t)
	id, _ := d.CreateProxyRule("example.com", "redirect", `{}`)

	// Another site must not delete this rule by ID (IDOR).
	if err := d.DeleteProxyRuleForSite(id, "attacker.com"); err == nil {
		t.Error("cross-site delete should fail")
	}
	if rules, _ := d.ListProxyRules("example.com"); len(rules) != 1 {
		t.Fatalf("rule removed by cross-site delete: got %d, want 1", len(rules))
	}

	if err := d.DeleteProxyRuleForSite(id, "example.com"); err != nil {
		t.Fatalf("DeleteProxyRuleForSite: %v", err)
	}
	if rules, _ := d.ListProxyRules("example.com"); len(rules) != 0 {
		t.Errorf("got %d, want 0", len(rules))
	}
}

func TestBlockUnblockIP(t *testing.T) {
	d := openTestDB(t)
	d.BlockIP("example.com", "1.2.3.4")
	rules, _ := d.ListIPRules("example.com")
	if len(rules) != 1 {
		t.Errorf("got %d, want 1", len(rules))
	}
	d.UnblockIP("example.com", "1.2.3.4")
	rules, _ = d.ListIPRules("example.com")
	if len(rules) != 0 {
		t.Errorf("got %d, want 0", len(rules))
	}
}

func TestFTPAccount(t *testing.T) {
	d := openTestDB(t)
	d.CreateFTPAccount("example.com", "ftpuser", "pass123")
	accounts, _ := d.ListFTPAccounts("example.com")
	if len(accounts) != 1 {
		t.Errorf("got %d, want 1", len(accounts))
	}
	if accounts[0].Username != "ftpuser" {
		t.Errorf("got %q", accounts[0].Username)
	}

	// Another site must not delete this account by username (IDOR).
	if err := d.DeleteFTPAccountForSite("attacker.com", "ftpuser"); err == nil {
		t.Error("cross-site FTP delete should fail")
	}
	if a, _ := d.ListFTPAccounts("example.com"); len(a) != 1 {
		t.Fatalf("account removed by cross-site delete: got %d, want 1", len(a))
	}

	if err := d.DeleteFTPAccountForSite("example.com", "ftpuser"); err != nil {
		t.Fatalf("DeleteFTPAccountForSite: %v", err)
	}
	accounts, _ = d.ListFTPAccounts("example.com")
	if len(accounts) != 0 {
		t.Errorf("got %d, want 0", len(accounts))
	}
}

func TestSSHKeys(t *testing.T) {
	d := openTestDB(t)
	d.AddSSHKey("mykey", "ssh-rsa AAAA...")
	keys, _ := d.ListSSHKeys()
	if len(keys) != 1 {
		t.Errorf("got %d, want 1", len(keys))
	}
	d.DeleteSSHKey("mykey")
	keys, _ = d.ListSSHKeys()
	if len(keys) != 0 {
		t.Errorf("got %d, want 0", len(keys))
	}
}
