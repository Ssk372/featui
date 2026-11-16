package model

import (
	"testing"
	"time"
)

func TestMergePreservesLocalState(t *testing.T) {
	unlocked := time.Date(2026, 11, 1, 20, 30, 0, 0, time.UTC)
	local := GameProgress{
		AppID: 49520,
		Title: "Borderlands 2",
		Achievements: []Achievement{
			{APIName: "Achievement_1", DisplayName: "Local uno", Completed: true, UnlockedAt: &unlocked, Source: SourceManual},
			{APIName: "Achievement_9", DisplayName: "Solo local"},
		},
	}
	schema := []Achievement{
		// El esquema llega con Completed=true a propósito: debe limpiarse.
		{APIName: "Achievement_1", DisplayName: "Nuevo nombre", Completed: true, Source: "steam"},
		{APIName: "Achievement_2", DisplayName: "Nuevo"},
	}

	now := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
	merged := Merge(local, schema, "", now)

	if merged.Title != "Borderlands 2" {
		t.Fatalf("title = %q, quiero el local", merged.Title)
	}
	if !merged.LastUpdated.Equal(now) {
		t.Fatalf("last_updated = %v, quiero %v", merged.LastUpdated, now)
	}
	if got := merged.Total(); got != 3 {
		t.Fatalf("total = %d, quiero 3", got)
	}

	first := merged.Achievements[0]
	if !first.Completed {
		t.Fatal("el logro local debe seguir completado")
	}
	if first.UnlockedAt == nil || !first.UnlockedAt.Equal(unlocked) {
		t.Fatal("unlocked_at local debe conservarse")
	}
	if first.Source != SourceManual {
		t.Fatalf("source = %q, quiero %q", first.Source, SourceManual)
	}
	if first.DisplayName != "Nuevo nombre" {
		t.Fatalf("display_name = %q, quiero el del esquema", first.DisplayName)
	}

	second := merged.Achievements[1]
	if second.Completed || second.UnlockedAt != nil || second.Source != "" {
		t.Fatal("un logro nuevo debe entrar pendiente y sin auditoría")
	}

	third := merged.Achievements[2]
	if third.APIName != "Achievement_9" {
		t.Fatalf("el logro local huérfano debe conservarse, got %q", third.APIName)
	}
}

func TestMergeEmptySchemaKeepsLocal(t *testing.T) {
	local := GameProgress{
		AppID: 1,
		Achievements: []Achievement{
			{APIName: "A", Completed: true},
		},
	}
	merged := Merge(local, nil, "Juego", time.Now())

	if merged.Total() != 1 || !merged.Achievements[0].Completed {
		t.Fatal("sin esquema no se debe perder el progreso local")
	}
}
