package model

import (
	"image"
	"strconv"
	"testing"

	"github.com/charmbracelet/crush/internal/ui/chat"
	"github.com/charmbracelet/crush/internal/ui/common"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/stretchr/testify/require"
)

// newScrollableChatUI returns a UI whose chat holds far more content than
// fits, drawn once so the scrollbar hit geometry is recorded.
func newScrollableChatUI(t *testing.T) *UI {
	t.Helper()

	u := newTestUI()
	msgs := make([]chat.MessageItem, 0, 200)
	for i := range 200 {
		msgs = append(msgs, testMessageItem{id: "m-" + strconv.Itoa(i), text: "message " + strconv.Itoa(i)})
	}
	u.chat.SetMessages(msgs...)
	u.updateLayoutAndSize()
	u.chat.ScrollToTop()
	u.chat.Draw(uv.NewScreenBuffer(u.width, u.height), u.layout.main)

	require.True(t, u.chat.scrollbarHit.ok, "chat should have a grabbable scrollbar")
	return u
}

// Dragging the thumb to the bottom of the track has to reach the bottom of
// the content, and dragging it back reaches the top. Anything less means the
// user cannot get where the thumb says they can.
func TestChatScrollbarDragCoversTheWholeRange(t *testing.T) {
	t.Parallel()

	u := newScrollableChatUI(t)
	hit := u.chat.scrollbarHit

	handled, _ := u.chat.HandleScrollbarDown(hit.col, hit.geom.ThumbPos)
	require.True(t, handled, "press on the thumb should start a drag")
	require.True(t, u.chat.ScrollbarDragging())

	u.chat.HandleScrollbarDrag(hit.height - 1)
	require.True(t, u.chat.AtBottom(), "dragging the thumb to the end should reach the bottom")

	u.chat.HandleScrollbarDrag(0)
	require.Equal(t, 0, u.chat.list.Offset(), "dragging back should reach the top")

	require.True(t, u.chat.EndScrollbarDrag())
	require.False(t, u.chat.ScrollbarDragging())
}

// Rows past either end of the track are clamped, not ignored: a drag that
// overshoots pins to the edge instead of freezing.
func TestChatScrollbarDragClampsOutsideTheTrack(t *testing.T) {
	t.Parallel()

	u := newScrollableChatUI(t)
	hit := u.chat.scrollbarHit

	u.chat.HandleScrollbarDown(hit.col, hit.geom.ThumbPos)
	u.chat.HandleScrollbarDrag(hit.height * 4)
	require.True(t, u.chat.AtBottom(), "overshooting downwards should pin to the bottom")

	u.chat.HandleScrollbarDrag(-hit.height)
	require.Equal(t, 0, u.chat.list.Offset(), "overshooting upwards should pin to the top")
}

// Pressing the bare track jumps there rather than paging towards it, and the
// press grabs the thumb by its middle so the following drag is smooth.
func TestChatScrollbarTrackPressJumps(t *testing.T) {
	t.Parallel()

	u := newScrollableChatUI(t)
	hit := u.chat.scrollbarHit
	require.Equal(t, 0, u.chat.list.Offset(), "test starts at the top")

	target := hit.height / 2
	handled, _ := u.chat.HandleScrollbarDown(hit.col, target)
	require.True(t, handled)

	// The list scrolls by whole lines and refuses to come to rest inside the
	// gap between two items, so the landing offset can differ from the exact
	// one the thumb row maps to by up to the gap. What matters is that the
	// press lands at the cursor rather than paging a screen towards it.
	want := hit.geom.OffsetForThumbPos(target - hit.geom.ThumbSize/2)
	require.InDelta(t, want, u.chat.list.Offset(), float64(u.chat.list.Gap()),
		"press on the track should jump the view there")
}

// The bar is one cell wide, which no one can aim at, so the grab zone is
// widened by scrollbarGrabSlack columns to its left and runs off the right
// edge. Beyond that the chat keeps its clicks.
func TestChatScrollbarGrabZone(t *testing.T) {
	t.Parallel()

	u := newScrollableChatUI(t)
	hit := u.chat.scrollbarHit
	row := hit.geom.ThumbPos

	for _, x := range []int{hit.col - scrollbarGrabSlack, hit.col, hit.col + 2} {
		u.chat.EndScrollbarDrag()
		handled, _ := u.chat.HandleScrollbarDown(x, row)
		require.True(t, handled, "column %d should be inside the grab zone", x)
	}

	u.chat.EndScrollbarDrag()
	handled, _ := u.chat.HandleScrollbarDown(hit.col-scrollbarGrabSlack-1, row)
	require.False(t, handled, "further left than the slack belongs to the chat")
	require.False(t, u.chat.ScrollbarDragging())

	handled, _ = u.chat.HandleScrollbarDown(hit.col, hit.height)
	require.False(t, handled, "the row below the track is not part of the bar")
}

// The bar auto-hides two seconds after a scroll, but the column stays
// grabbable: terminals report motion only while a button is held, so a bar
// that could only be grabbed while painted could not be aimed at.
func TestChatScrollbarGrabbableWhileHidden(t *testing.T) {
	t.Parallel()

	u := newScrollableChatUI(t)
	u.chat.scrollbarVisible = false
	u.chat.Draw(uv.NewScreenBuffer(u.width, u.height), u.layout.main)

	hit := u.chat.scrollbarHit
	require.True(t, hit.ok, "the hit geometry survives the bar being hidden")

	handled, _ := u.chat.HandleScrollbarDown(hit.col, hit.geom.ThumbPos)
	require.True(t, handled)
}

// The sidebar's bar is driven by the same geometry as the chat's, but over
// absolute screen coordinates and a plain line offset rather than the list.
func TestSidebarScrollbarDrag(t *testing.T) {
	t.Parallel()

	const (
		trackTop      = 5
		trackHeight   = 20
		col           = 100
		totalLines    = 200
		contentHeight = trackHeight
	)

	u := newTestUI()
	u.sidebarMaxOffsetVal = totalLines - contentHeight
	u.sidebarScrollbarRect = image.Rect(col, trackTop, col+1, trackTop+trackHeight)
	u.sidebarScrollbarGeom, u.sidebarScrollbarOK = common.ScrollbarLayout(
		trackHeight, totalLines, contentHeight, 0)
	require.True(t, u.sidebarScrollbarOK)

	require.Nil(t, u.sidebarScrollbarDown(col-scrollbarGrabSlack-1, trackTop),
		"presses left of the grab zone are not ours")
	require.False(t, u.sidebarScrollbarDrag)

	u.sidebarScrollbarDown(col, trackTop+u.sidebarScrollbarGeom.ThumbPos)
	require.True(t, u.sidebarScrollbarDrag)

	u.sidebarScrollbarMove(trackTop + trackHeight - 1)
	require.Equal(t, u.sidebarMaxOffsetVal, u.sidebarOffset, "the bottom of the track is the bottom of the content")

	u.sidebarScrollbarMove(trackTop - 100)
	require.Equal(t, 0, u.sidebarOffset, "overshooting upwards pins to the top")
}

// A hide timer that fires mid-drag must not pull the bar out from under the
// cursor.
func TestChatScrollbarStaysVisibleWhileDragging(t *testing.T) {
	t.Parallel()

	u := newScrollableChatUI(t)
	hit := u.chat.scrollbarHit

	u.chat.HandleScrollbarDown(hit.col, hit.geom.ThumbPos)
	u.chat.scrollbarVisible = true
	u.chat.HideScrollbar(u.chat.scrollbarHideSeq)
	require.True(t, u.chat.scrollbarVisible, "the bar must stay up for the duration of the drag")
}
