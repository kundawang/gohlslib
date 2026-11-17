package gohlslib

import (
	"errors"
	"fmt"
	"time"

	"github.com/bluenviron/gohlslib/v2/pkg/playlist"
)

// ErrClientDateTimeNotFound is returned when the playlist does not contain
// the wall-clock time provided with Client.StartDateTime.
var ErrClientDateTimeNotFound = errors.New("requested start time not found in playlist")

// segmentStartTimes computes the wall-clock start time of each segment of a media playlist.
// Segments with EXT-X-PROGRAM-DATE-TIME are anchors; the start time of adjacent segments
// without the tag is derived from segment durations. Derivation does not cross
// EXT-X-DISCONTINUITY, since the timeline across a discontinuity is unknown.
// Segments whose start time cannot be computed get a nil entry.
func segmentStartTimes(segments []*playlist.MediaSegment) []*time.Time {
	ret := make([]*time.Time, len(segments))

	for i, seg := range segments {
		if seg.DateTime != nil {
			ret[i] = new(*seg.DateTime)
		}
	}

	for i := 1; i < len(segments); i++ {
		if ret[i] == nil && ret[i-1] != nil && !segments[i].Discontinuity {
			ret[i] = new(ret[i-1].Add(segments[i-1].Duration))
		}
	}

	for i := len(segments) - 2; i >= 0; i-- {
		if ret[i] == nil && ret[i+1] != nil && !segments[i+1].Discontinuity {
			ret[i] = new(ret[i+1].Add(-segments[i].Duration))
		}
	}

	return ret
}

// findSegmentWithDateTime finds the segment or part that contains the given wall-clock time.
// It returns the index of the segment and its (possibly derived) start time.
// When the time falls inside the in-progress segment of a low-latency playlist,
// it returns a segment index equal to len(segments), the start time of the part
// and the index of the part inside the playlist part list.
func findSegmentWithDateTime(
	segments []*playlist.MediaSegment,
	parts []*playlist.MediaPart,
	target time.Time,
) (int, *time.Time, int, error) {
	starts := segmentStartTimes(segments)

	firstKnown := -1
	lastKnown := -1
	for i, start := range starts {
		if start != nil {
			if firstKnown == -1 {
				firstKnown = i
			}
			lastKnown = i
		}
	}

	if firstKnown == -1 {
		return 0, nil, 0, fmt.Errorf("%w: playlist has no EXT-X-PROGRAM-DATE-TIME", ErrClientDateTimeNotFound)
	}

	if target.Before(*starts[firstKnown]) {
		return 0, nil, 0, fmt.Errorf(
			"%w: time is before the start of the playlist window", ErrClientDateTimeNotFound)
	}

	for i := firstKnown; i <= lastKnown; i++ {
		if starts[i] == nil {
			// segment on an unknown timeline (across a discontinuity):
			// it cannot be matched against the target.
			continue
		}

		if target.Before(*starts[i]) {
			return 0, nil, 0, fmt.Errorf(
				"%w: time falls inside a period without timing information", ErrClientDateTimeNotFound)
		}

		if target.Before(starts[i].Add(segments[i].Duration)) {
			return i, starts[i], -1, nil
		}
	}

	// check parts of the in-progress segment (low-latency playlists)
	if lastKnown == len(segments)-1 {
		partStart := starts[lastKnown].Add(segments[lastKnown].Duration)

		for i, part := range parts {
			if target.Before(partStart.Add(part.Duration)) {
				return len(segments), new(partStart), i, nil
			}
			partStart = partStart.Add(part.Duration)
		}
	}

	return 0, nil, 0, fmt.Errorf(
		"%w: time is after the end of the playlist window", ErrClientDateTimeNotFound)
}
