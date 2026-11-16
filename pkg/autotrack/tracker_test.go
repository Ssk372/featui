package autotrack

import (
	"testing"
	"time"
)

func TestFirstSnapshotSeedsWithoutUnlocking(t *testing.T) {
	tracker := New()
	current := map[string]State{
		"A": {Unlocked: true},
		"B": {Unlocked: true},
		"C": {Unlocked: false},
	}

	if unlocks := tracker.Diff(current); len(unlocks) != 0 {
		t.Fatalf("el primer snapshot no debe desbloquear nada, got %+v", unlocks)
	}
	if !tracker.Seeded() {
		t.Fatal("el tracker debería quedar sembrado")
	}
}

func TestDetectsTransitionWithSteamTime(t *testing.T) {
	tracker := New()
	steamTime := time.Date(2026, 11, 15, 18, 30, 0, 0, time.UTC)

	tracker.Diff(map[string]State{
		"A": {Unlocked: false},
		"B": {Unlocked: false},
	})

	unlocks := tracker.Diff(map[string]State{
		"A": {Unlocked: true, UnlockedAt: steamTime},
		"B": {Unlocked: false},
	})

	if len(unlocks) != 1 || unlocks[0].APIName != "A" {
		t.Fatalf("unlocks = %+v, quiero solo A", unlocks)
	}
	if !unlocks[0].UnlockedAt.Equal(steamTime) {
		t.Fatalf("unlocked_at = %v, quiero %v", unlocks[0].UnlockedAt, steamTime)
	}
}

func TestDoesNotRepeatSameUnlock(t *testing.T) {
	tracker := New()
	tracker.Diff(map[string]State{"A": {Unlocked: false}})

	if got := tracker.Diff(map[string]State{"A": {Unlocked: true}}); len(got) != 1 {
		t.Fatalf("primera transición: %+v", got)
	}
	if got := tracker.Diff(map[string]State{"A": {Unlocked: true}}); len(got) != 0 {
		t.Fatalf("no debe repetir el desbloqueo: %+v", got)
	}
}

// Simula: Steam ya lo tenía, el usuario lo desmarca a mano y vuelve el poll.
func TestManualUnmarkIsNotReverted(t *testing.T) {
	tracker := New()
	tracker.Diff(map[string]State{"A": {Unlocked: true}})

	if got := tracker.Diff(map[string]State{"A": {Unlocked: true}}); len(got) != 0 {
		t.Fatalf("sin transición no debe marcar nada: %+v", got)
	}
}

func TestAdoptReturnsAllUnlocked(t *testing.T) {
	current := map[string]State{
		"A": {Unlocked: true},
		"B": {Unlocked: false},
		"C": {Unlocked: true, UnlockedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
	}

	unlocks := Adopt(current)
	if len(unlocks) != 2 {
		t.Fatalf("unlocks = %d, quiero 2", len(unlocks))
	}
	if unlocks[0].APIName != "A" || unlocks[1].APIName != "C" {
		t.Fatalf("orden inesperado: %+v", unlocks)
	}
}
