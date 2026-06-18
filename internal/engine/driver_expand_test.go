package engine

import (
	"testing"

	"github.com/aystro/apod/internal/models"
)

// TestExpandDriverVariables asserts that EVERY driver string that can carry a
// ${variable} is expanded — not just service fields. A driver must be able to
// reference site variables (domain, db creds, paths) anywhere a command or URL
// is declared, regardless of stack, or it is not truly agnostic.
func TestExpandDriverVariables(t *testing.T) {
	d := &models.Driver{
		Services: map[string]models.DriverService{
			"app": {
				Image:       "img:${tag}",
				Volumes:     []string{"${site_root}:/app"},
				Environment: map[string]string{"DB": "${site_db_name}"},
				Command:     "run ${site_domain}",
			},
		},
		Setup: []models.DriverSetupStep{{Command: "setup ${site_domain}"}},
		Files: []models.DriverFile{{Path: "${site_root}/x", Content: "host=${site_domain}"}},
		Deploy: models.DriverDeployHooks{
			BeforeDeploy: []string{"composer ${site_domain}"},
			AfterDeploy:  []string{"migrate ${site_db_name}"},
		},
		Healthcheck: models.DriverHealthcheck{URL: "http://${site_domain}/health"},
		Cron:        []models.DriverCron{{Command: "cron ${site_domain}", Schedule: "* * * * *"}},
	}
	vars := map[string]string{
		"tag":          "1.2",
		"site_root":    "/srv/app",
		"site_domain":  "ex.com",
		"site_db_name": "ex_com",
	}

	ExpandDriverVariables(d, vars)

	checks := map[string]string{
		"service image":   d.Services["app"].Image,
		"service volume":  d.Services["app"].Volumes[0],
		"service env":     d.Services["app"].Environment["DB"],
		"service command": d.Services["app"].Command,
		"setup command":   d.Setup[0].Command,
		"file path":       d.Files[0].Path,
		"file content":    d.Files[0].Content,
		"before_deploy":   d.Deploy.BeforeDeploy[0],
		"after_deploy":    d.Deploy.AfterDeploy[0],
		"healthcheck url": d.Healthcheck.URL,
		"cron command":    d.Cron[0].Command,
	}
	want := map[string]string{
		"service image":   "img:1.2",
		"service volume":  "/srv/app:/app",
		"service env":     "ex_com",
		"service command": "run ex.com",
		"setup command":   "setup ex.com",
		"file path":       "/srv/app/x",
		"file content":    "host=ex.com",
		"before_deploy":   "composer ex.com",
		"after_deploy":    "migrate ex_com",
		"healthcheck url": "http://ex.com/health",
		"cron command":    "cron ex.com",
	}
	for name, got := range checks {
		if got != want[name] {
			t.Errorf("%s = %q, want %q", name, got, want[name])
		}
	}
}
