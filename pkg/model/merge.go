package model

import "time"

// Merge combina el esquema oficial de Steam con el progreso local.
//
// Reglas:
//   - El estado local (Completed, UnlockedAt, Source) siempre gana: Steam solo
//     aporta metadatos, nunca desbloquea ni bloquea nada por su cuenta.
//   - Los logros nuevos del esquema se añaden como pendientes.
//   - Los logros locales que ya no aparecen en el esquema se conservan al final
//     para no perder historial.
func Merge(local GameProgress, schema []Achievement, title string, now time.Time) GameProgress {
	merged := GameProgress{
		AppID:        local.AppID,
		Title:        title,
		Achievements: make([]Achievement, 0, len(schema)+len(local.Achievements)),
		LastUpdated:  now,
	}
	if merged.Title == "" {
		merged.Title = local.Title
	}

	previous := make(map[string]Achievement, len(local.Achievements))
	for _, a := range local.Achievements {
		previous[a.APIName] = a
	}

	seen := make(map[string]bool, len(schema))
	for _, fresh := range schema {
		// El esquema nunca describe estado local: se limpia por si viene de caché.
		fresh.Completed = false
		fresh.UnlockedAt = nil
		fresh.Source = ""

		if prev, ok := previous[fresh.APIName]; ok {
			fresh.Completed = prev.Completed
			fresh.UnlockedAt = prev.UnlockedAt
			fresh.Source = prev.Source
		}

		merged.Achievements = append(merged.Achievements, fresh)
		seen[fresh.APIName] = true
	}

	for _, old := range local.Achievements {
		if !seen[old.APIName] {
			merged.Achievements = append(merged.Achievements, old)
		}
	}

	return merged
}
