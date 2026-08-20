package bootstrap

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpenLogFileCreatesParentDirectories(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "runtime", "basetion.log")
	file, err := openLogFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString("runtime entry\n"); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "runtime entry\n" {
		t.Fatalf("log contents = %q", contents)
	}
}
