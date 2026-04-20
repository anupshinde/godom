package main

import (
	"sort"
	"time"

	"github.com/anupshinde/godom"
)

type ScoreEntry struct {
	Score int
	Date  string
	Rank  int
}

type Scores struct {
	godom.Island
	Entries []ScoreEntry
}

func NewScores() *Scores {
	return &Scores{}
}

func (s *Scores) Add(score int) {
	s.Entries = append(s.Entries, ScoreEntry{
		Score: score,
		Date:  time.Now().Format("Jan 2, 15:04"),
	})
	sort.Slice(s.Entries, func(i, j int) bool {
		return s.Entries[i].Score > s.Entries[j].Score
	})
	// Keep top 10
	if len(s.Entries) > 10 {
		s.Entries = s.Entries[:10]
	}
	// Update ranks
	for i := range s.Entries {
		s.Entries[i].Rank = i + 1
	}
	s.Refresh()
}
