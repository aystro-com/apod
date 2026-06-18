package engine

import (
	"path/filepath"
	"testing"

	"github.com/aystro/apod/internal/db"
	"github.com/aystro/apod/internal/models"
)

func TestSiteVars(t *testing.T) {
	d, err := db.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	e := NewWithDB(d)

	d.SetSiteSecret("ex.com", "db_password", "p@ss")
	d.SetSiteSecret("ex.com", "jwt_secret", "jjj")

	v := e.siteVars(&models.Site{Domain: "ex.com", Owner: ""})
	if v["site_domain"] != "ex.com" {
		t.Errorf("site_domain = %q", v["site_domain"])
	}
	if v["site_db_name"] != "ex_com" || v["site_db_user"] != "ex_com" {
		t.Errorf("db name/user = %q/%q, want ex_com", v["site_db_name"], v["site_db_user"])
	}
	if v["site_db_pass"] != "p@ss" {
		t.Errorf("site_db_pass = %q, want from secrets store", v["site_db_pass"])
	}
	if v["jwt_secret"] != "jjj" {
		t.Errorf("jwt_secret = %q, want from secrets store", v["jwt_secret"])
	}
}
