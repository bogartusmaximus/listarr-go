package arr_test

import (
	"testing"

	"github.com/bogartusmaximus/listarr-go/internal/arr"
)

func TestRegistryRegisterAndList(t *testing.T) {
	reg := arr.NewRegistry()
	c, err := arr.NewRadarr("http://127.0.0.1:7878", "k", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := reg.RegisterRadarr("local", c); err != nil {
		t.Fatal(err)
	}
	if err := reg.RegisterRadarr("local", c); err == nil {
		t.Fatal("expected duplicate error")
	}
	list := reg.List()
	if len(list) != 1 || list[0].Name != "local" || list[0].Kind != arr.KindRadarr {
		t.Fatalf("%+v", list)
	}
}

func TestLoadRegistryFromEnvLegacyAndNamed(t *testing.T) {
	t.Setenv("LISTARR_RADARR_URL", "http://127.0.0.1:7878")
	t.Setenv("LISTARR_RADARR_API_KEY", "rk")
	t.Setenv("LISTARR_ARR_REMOTE_URL", "http://127.0.0.1:7879")
	t.Setenv("LISTARR_ARR_REMOTE_API_KEY", "rk2")
	t.Setenv("LISTARR_ARR_REMOTE_KIND", "radarr")
	// clear sonarr
	t.Setenv("LISTARR_SONARR_URL", "")
	t.Setenv("LISTARR_SONARR_API_KEY", "")

	reg, err := arr.LoadRegistryFromEnv(nil)
	if err != nil {
		t.Fatal(err)
	}
	if reg.Len() != 2 {
		t.Fatalf("len=%d list=%+v", reg.Len(), reg.List())
	}
	if _, err := reg.Radarr("radarr"); err != nil {
		t.Fatal(err)
	}
	if _, err := reg.Radarr("remote"); err != nil {
		t.Fatal(err)
	}
}
