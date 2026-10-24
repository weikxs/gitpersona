package backup

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/weikxs/gitpersona/internal/i18n"
)

func TestBackupFailureIsChineseAndKeepsItsCause(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := Snapshot(filepath.Join(blocker, "backups"), fixedTime, nil)

	var pathErr *fs.PathError
	if !errors.As(err, &pathErr) {
		t.Fatalf("Snapshot() = %v, want a *fs.PathError in the chain", err)
	}
	if got := i18n.Text(i18n.ZH, err); !strings.HasPrefix(got, "创建备份目录失败：") {
		t.Errorf("Chinese text = %q", got)
	}
	if !strings.HasPrefix(err.Error(), "create backup directory: ") {
		t.Errorf("English text = %q", err.Error())
	}
}

func TestReadingAMissingManifestIsChineseAndKeepsItsCause(t *testing.T) {
	_, err := ReadManifest(t.TempDir())

	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("ReadManifest() = %v, want fs.ErrNotExist in the chain", err)
	}
	if got := i18n.Text(i18n.ZH, err); !strings.HasPrefix(got, "读取备份清单失败：") {
		t.Errorf("Chinese text = %q", got)
	}
}

func TestRestoringFromAFolderWithoutAManifestIsChinese(t *testing.T) {
	err := Restore(t.TempDir())

	if err == nil {
		t.Fatal("Restore() = nil, want an error")
	}
	if got := i18n.Text(i18n.ZH, err); strings.Contains(got, "read backup manifest") {
		t.Errorf("Chinese text still has English: %q", got)
	}
}
