package comrad

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestRealLlamaCppE2E(t *testing.T) {
	if os.Getenv("COMRAD_E2E_REAL_LLAMA") != "1" {
		t.Skip("set COMRAD_E2E_REAL_LLAMA=1 to run the real llama.cpp/GGUF e2e test")
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(filepath.Join(root, "scripts", "e2e-real-llama.sh"))
	cmd.Dir = root
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), "COMRAD_E2E_KEEP_RUNNING=0")
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
}
