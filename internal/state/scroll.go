// Package state provides scroll state and persistence for claude-tmux.
package state

// ScrollState implements center-locked scrolling for a list.
// The selected item stays centered in the viewport.
type ScrollState struct {
	Selected   int
	Offset     int
	Total      int
	ViewHeight int
}

// NewScrollState creates a new scroll state.
func NewScrollState() ScrollState {
	return ScrollState{
		Selected: 0,
		Offset:   0,
	}
}

// SetTotal updates the total item count and clamps selection.
func (s *ScrollState) SetTotal(total int) {
	s.Total = total
	if s.Selected >= total {
		s.Selected = MaxInt(0, total-1)
	}
	s.recalcOffset()
}

// SetViewHeight updates the viewport height.
func (s *ScrollState) SetViewHeight(h int) {
	s.ViewHeight = h
	s.recalcOffset()
}

// MoveUp moves selection up by one.
func (s *ScrollState) MoveUp() {
	if s.Selected > 0 {
		s.Selected--
		s.recalcOffset()
	}
}

// MoveDown moves selection down by one.
func (s *ScrollState) MoveDown() {
	if s.Selected < s.Total-1 {
		s.Selected++
		s.recalcOffset()
	}
}

// SetSelected sets the selection to a specific index and recalculates offset.
func (s *ScrollState) SetSelected(idx int) {
	if idx >= 0 && idx < s.Total {
		s.Selected = idx
		s.recalcOffset()
	}
}

// recalcOffset keeps the selected item centered.
func (s *ScrollState) recalcOffset() {
	if s.ViewHeight <= 0 || s.Total <= 0 {
		s.Offset = 0
		return
	}

	// Center the selected item
	center := s.ViewHeight / 2
	idealOffset := s.Selected - center

	// Clamp
	maxOffset := s.Total - s.ViewHeight
	if maxOffset < 0 {
		maxOffset = 0
	}

	if idealOffset < 0 {
		idealOffset = 0
	}
	if idealOffset > maxOffset {
		idealOffset = maxOffset
	}

	s.Offset = idealOffset
}

// VisibleRange returns the start and end indices of visible items.
func (s *ScrollState) VisibleRange() (int, int) {
	start := s.Offset
	end := s.Offset + s.ViewHeight
	if end > s.Total {
		end = s.Total
	}
	return start, end
}

// IsSelected returns true if the given index is the selected item.
func (s *ScrollState) IsSelected(idx int) bool {
	return idx == s.Selected
}
