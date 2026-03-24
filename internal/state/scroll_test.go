package state

import (
	"testing"
)

func TestScrollState_SetTotal(t *testing.T) {
	s := NewScrollState()
	s.SetTotal(10)

	if s.Total != 10 {
		t.Errorf("Total = %d, want 10", s.Total)
	}
	if s.Selected != 0 {
		t.Errorf("Selected = %d, want 0", s.Selected)
	}
}

func TestScrollState_MoveDown(t *testing.T) {
	s := NewScrollState()
	s.SetTotal(5)
	s.SetViewHeight(3)

	s.MoveDown()
	if s.Selected != 1 {
		t.Errorf("Selected = %d, want 1", s.Selected)
	}

	// Move to end
	for i := 0; i < 10; i++ {
		s.MoveDown()
	}
	if s.Selected != 4 {
		t.Errorf("Selected = %d, want 4 (clamped to last item)", s.Selected)
	}
}

func TestScrollState_MoveUp(t *testing.T) {
	s := NewScrollState()
	s.SetTotal(5)
	s.SetViewHeight(3)
	s.Selected = 4

	s.MoveUp()
	if s.Selected != 3 {
		t.Errorf("Selected = %d, want 3", s.Selected)
	}

	// Move to start
	for i := 0; i < 10; i++ {
		s.MoveUp()
	}
	if s.Selected != 0 {
		t.Errorf("Selected = %d, want 0 (clamped to first item)", s.Selected)
	}
}

func TestScrollState_VisibleRange(t *testing.T) {
	s := NewScrollState()
	s.SetTotal(10)
	s.SetViewHeight(3)

	start, end := s.VisibleRange()
	if start != 0 || end != 3 {
		t.Errorf("VisibleRange() = (%d, %d), want (0, 3)", start, end)
	}

	// Move down to center
	s.Selected = 5
	s.recalcOffset()
	start, end = s.VisibleRange()
	// With center-lock, selected=5 and viewHeight=3, offset should be 5-1=4
	if start != 4 || end != 7 {
		t.Errorf("VisibleRange() = (%d, %d), want (4, 7)", start, end)
	}
}

func TestScrollState_SetSelected(t *testing.T) {
	s := NewScrollState()
	s.SetTotal(10)

	s.SetSelected(5)
	if s.Selected != 5 {
		t.Errorf("Selected = %d, want 5", s.Selected)
	}

	// Out of bounds should not change selection
	s.SetSelected(15)
	if s.Selected != 5 {
		t.Errorf("Selected = %d, want 5 (unchanged due to out of bounds)", s.Selected)
	}
}
