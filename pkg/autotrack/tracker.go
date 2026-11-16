// Package autotrack decide qué logros se desbloquearon comparando snapshots
// sucesivos de Steam.
//
// Regla clave: el primer snapshot solo establece una línea base. Así, abrir un
// juego que ya está al 100% en Steam no desbloquea nada de golpe, y un
// desmarque manual no se revierte en el siguiente sondeo.
package autotrack

import (
	"sort"
	"time"
)

// State es el estado de un logro en Steam.
type State struct {
	Unlocked   bool
	UnlockedAt time.Time
}

// Unlock es una transición de pendiente a desbloqueado.
type Unlock struct {
	APIName    string
	UnlockedAt time.Time
}

// Tracker guarda el último snapshot conocido de Steam.
type Tracker struct {
	previous map[string]bool
	seeded   bool
}

// New crea un tracker sin línea base.
func New() *Tracker {
	return &Tracker{previous: make(map[string]bool)}
}

// Seeded indica si ya se tomó el primer snapshot.
func (t *Tracker) Seeded() bool { return t != nil && t.seeded }

// Reset olvida el snapshot anterior; el siguiente Diff volverá a sembrar.
func (t *Tracker) Reset() {
	if t == nil {
		return
	}
	t.previous = make(map[string]bool)
	t.seeded = false
}

// Diff devuelve las transiciones false→true respecto al snapshot anterior.
// El primer Diff siembra la línea base y devuelve nil.
func (t *Tracker) Diff(current map[string]State) []Unlock {
	if t == nil {
		return nil
	}
	if t.previous == nil {
		t.previous = make(map[string]bool)
	}

	if !t.seeded {
		for name, state := range current {
			t.previous[name] = state.Unlocked
		}
		t.seeded = true
		return nil
	}

	now := time.Now()
	unlocks := make([]Unlock, 0)
	for name, state := range current {
		if state.Unlocked && !t.previous[name] {
			at := state.UnlockedAt
			if at.IsZero() {
				at = now
			}
			unlocks = append(unlocks, Unlock{APIName: name, UnlockedAt: at})
		}
	}

	for name, state := range current {
		t.previous[name] = state.Unlocked
	}

	sort.Slice(unlocks, func(i, j int) bool { return unlocks[i].APIName < unlocks[j].APIName })
	return unlocks
}

// Adopt devuelve todos los logros desbloqueados del snapshot actual. Se usa en
// la sincronización manual (tecla S), que el usuario confirma expresamente.
func Adopt(current map[string]State) []Unlock {
	now := time.Now()
	unlocks := make([]Unlock, 0, len(current))
	for name, state := range current {
		if !state.Unlocked {
			continue
		}
		at := state.UnlockedAt
		if at.IsZero() {
			at = now
		}
		unlocks = append(unlocks, Unlock{APIName: name, UnlockedAt: at})
	}
	sort.Slice(unlocks, func(i, j int) bool { return unlocks[i].APIName < unlocks[j].APIName })
	return unlocks
}
