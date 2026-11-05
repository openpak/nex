package nexhost

import (
	"strings"
	"testing"
)

func TestCoreStorageFlag(t *testing.T) {
	t.Setenv("NEX_CORE_STORAGE", "")
	if CoreStorage(CoreRankingsStore) {
		t.Fatal("off by default")
	}
	t.Setenv("NEX_CORE_STORAGE", " DataStore , rankings")
	if !CoreStorage(CoreRankingsStore) || !CoreStorage(CoreDataStoreStore) || CoreStorage(CoreMessagingStore) {
		t.Fatal("list parsing")
	}
}

func TestCoreRankingsRefusesMissingSettings(t *testing.T) {
	for _, v := range []string{"NEX_LEADERBOARDS_ADDR", "NEX_LEADERBOARDS_KEY", "LEADERBOARDS_INTERNAL_KEY"} {
		t.Setenv(v, "")
	}
	_, err := NewCoreRankings(Config{}, CoreTitle{Key: "t", Network: "wiiu"}, nil)
	if err == nil || !strings.Contains(err.Error(), "NEX_LEADERBOARDS_ADDR") {
		t.Fatalf("want a loud error, got %v", err)
	}
	t.Setenv("NEX_LEADERBOARDS_ADDR", "127.0.0.1:1")
	t.Setenv("LEADERBOARDS_INTERNAL_KEY", "k")
	t.Setenv("NEX_TITLE_STORAGE_ADDR", "127.0.0.1:1")
	t.Setenv("TITLE_STORAGE_INTERNAL_KEY", "k")
	_, err = NewCoreRankings(Config{}, CoreTitle{Key: "t", Network: "wiiu"}, nil)
	if err == nil || !strings.Contains(err.Error(), "NEX_ACCOUNT_GRPC_ADDR") {
		t.Fatalf("want the nn-account error, got %v", err)
	}
}

func TestCoreDataStoreRefusesMissingSettings(t *testing.T) {
	for _, v := range []string{"NEX_TITLE_STORAGE_ADDR", "NEX_TITLE_STORAGE_KEY", "TITLE_STORAGE_INTERNAL_KEY"} {
		t.Setenv(v, "")
	}
	_, err := NewCoreDataStore(Config{}, CoreTitle{Key: "t", Network: "wiiu"}, 0, nil)
	if err == nil || !strings.Contains(err.Error(), "NEX_TITLE_STORAGE_ADDR") {
		t.Fatalf("want a loud error, got %v", err)
	}
	t.Setenv("NEX_TITLE_STORAGE_ADDR", "127.0.0.1:1")
	t.Setenv("TITLE_STORAGE_INTERNAL_KEY", "k")
	_, err = NewCoreDataStore(Config{}, CoreTitle{Key: "t", Network: "wiiu"}, 0, nil)
	if err == nil || !strings.Contains(err.Error(), "NEX_ACCOUNT_GRPC_ADDR") {
		t.Fatalf("want the nn-account error, got %v", err)
	}
}
