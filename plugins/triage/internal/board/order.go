package board

import "sort"

// Group is one rendered unit: a batch with its members, or a single
// non-batched comment carrying a nil Batch.
type Group struct {
	Batch    *Batch
	Comments []Comment
}

// Order arranges a briefing into presentation order: batches first,
// ordered by their highest-severity member with ties broken by batch key
// ascending, then non-batched comments ordered by severity with ties
// broken by id ascending.
//
// This is the order the triage loop presents in. The board and the
// conversation render one queue, so a developer who moves between them
// does not see the work reshuffled.
func Order(b Briefing) []Group {
	byKey := map[string]*Batch{}
	for i := range b.Batches {
		byKey[b.Batches[i].BatchKey] = &b.Batches[i]
	}

	members := map[string][]Comment{}
	var loose []Comment
	for _, c := range b.Comments {
		if c.BatchKey != "" {
			members[c.BatchKey] = append(members[c.BatchKey], c)
			continue
		}
		loose = append(loose, c)
	}

	batched := make([]Group, 0, len(members))
	for key, cs := range members {
		sort.SliceStable(cs, func(i, j int) bool {
			if severityRank[cs[i].Severity] != severityRank[cs[j].Severity] {
				return severityRank[cs[i].Severity] < severityRank[cs[j].Severity]
			}
			return cs[i].ID < cs[j].ID
		})
		batched = append(batched, Group{Batch: byKey[key], Comments: cs})
	}
	sort.SliceStable(batched, func(i, j int) bool {
		hi, hj := highestSeverity(batched[i].Comments), highestSeverity(batched[j].Comments)
		if hi != hj {
			return hi < hj
		}
		return batched[i].Batch.BatchKey < batched[j].Batch.BatchKey
	})

	sort.SliceStable(loose, func(i, j int) bool {
		if severityRank[loose[i].Severity] != severityRank[loose[j].Severity] {
			return severityRank[loose[i].Severity] < severityRank[loose[j].Severity]
		}
		return loose[i].ID < loose[j].ID
	})

	out := batched
	for _, c := range loose {
		out = append(out, Group{Comments: []Comment{c}})
	}
	return out
}

// highestSeverity returns the rank of the most severe member, which is
// what orders one batch against another.
func highestSeverity(cs []Comment) int {
	best := len(severityRank)
	for _, c := range cs {
		if r, ok := severityRank[c.Severity]; ok && r < best {
			best = r
		}
	}
	return best
}
