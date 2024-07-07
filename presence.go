package supabase_realtime_go

import (
	"errors"
)

type RealtimePresence struct {
	state        map[string][]Presence
	pendingDiffs []RawPresenceDiff
	joinRef      string
	caller       PresenceCaller
	channel      *Channel
}

type Presence struct {
	PresenceRef string
	Metas       []map[string]interface{}
}

type RawPresenceState map[string]Presence

type PresenceDiff struct {
	Joins  map[string][]Presence
	Leaves map[string][]Presence
}

type RawPresenceDiff struct {
	Joins  RawPresenceState
	Leaves RawPresenceState
}

type PresenceCaller struct {
	OnJoin  func(key string, currentPresences, newPresences []Presence)
	OnLeave func(key string, currentPresences, leftPresences []Presence)
	OnSync  func()
}

func NewRealtimePresence(channel *Channel) *RealtimePresence {
	rp := &RealtimePresence{
		state:        make(map[string][]Presence),
		pendingDiffs: make([]RawPresenceDiff, 0),
		joinRef:      "",
		caller:       PresenceCaller{},
		channel:      channel,
	}

	events := map[string]string{
		"state": "presence_state",
		"diff":  "presence_diff",
	}

	channel.On(events["state"], func(payload interface{}) {
		rp.joinRef = channel.joinRef()
		newState := payload.(RawPresenceState)
		rp.state = syncState(rp.state, newState, rp.caller.OnJoin, rp.caller.OnLeave)

		for _, diff := range rp.pendingDiffs {
			rp.state = syncDiff(rp.state, PresenceDiff{
				Joins:  transformState(diff.Joins),
				Leaves: transformState(diff.Leaves),
			}, rp.caller.OnJoin, rp.caller.OnLeave)
		}

		rp.pendingDiffs = []RawPresenceDiff{}
		rp.caller.OnSync()
	})

	channel.On(events["diff"], func(payload interface{}) {
		diff := payload.(RawPresenceDiff)
		if rp.inPendingSyncState() {
			rp.pendingDiffs = append(rp.pendingDiffs, diff)
		} else {
			rp.state = syncDiff(rp.state, PresenceDiff{
				Joins:  transformState(diff.Joins),
				Leaves: transformState(diff.Leaves),
			}, rp.caller.OnJoin, rp.caller.OnLeave)
			rp.caller.OnSync()
		}
	})

	rp.onJoin(func(key string, currentPresences, newPresences []Presence) {
		channel.trigger("presence", map[string]interface{}{
			"event":            "join",
			"key":              key,
			"currentPresences": currentPresences,
			"newPresences":     newPresences,
		})
	})

	rp.onLeave(func(key string, currentPresences, leftPresences []Presence) {
		channel.trigger("presence", map[string]interface{}{
			"event":            "leave",
			"key":              key,
			"currentPresences": currentPresences,
			"leftPresences":    leftPresences,
		})
	})

	rp.onSync(func() {
		channel.trigger("presence", map[string]interface{}{
			"event": "sync",
		})
	})

	return rp
}

func (rp *RealtimePresence) Track(payload map[string]interface{}, opts map[string]interface{}) error {
	if rp.channel.state != CHANNEL_STATE_JOINED {
		return errors.New("channel is not joined")
	}

	// Your tracking logic here. For simplicity, we will assume a basic track operation
	rp.channel.push("presence_diff", payload, rp.channel.timeout).Send()
	return nil
}

func syncState(
	currentState map[string][]Presence,
	newState RawPresenceState,
	onJoin func(key string, currentPresences, newPresences []Presence),
	onLeave func(key string, currentPresences, leftPresences []Presence),
) map[string][]Presence {
	state := cloneDeep(currentState)
	joins := make(map[string][]Presence)
	leaves := make(map[string][]Presence)

	for key, presences := range state {
		if _, ok := newState[key]; !ok {
			leaves[key] = presences
		}
	}

	for key, newPresences := range newState {
		currentPresences, ok := state[key]
		if ok {
			newPresenceRefs := make(map[string]bool)
			for _, np := range newPresences.Metas {
				newPresenceRefs[np["presence_ref"].(string)] = true
			}

			curPresenceRefs := make(map[string]bool)
			for _, cp := range currentPresences {
				for _, meta := range cp.Metas {
					curPresenceRefs[meta["presence_ref"].(string)] = true
				}
			}

			joinedPresences := []Presence{}
			for _, np := range newPresences.Metas {
				if !curPresenceRefs[np["presence_ref"].(string)] {
					joinedPresences = append(joinedPresences, Presence{PresenceRef: np["presence_ref"].(string), Metas: []map[string]interface{}{np}})
				}
			}

			leftPresences := []Presence{}
			for _, cp := range currentPresences {
				for _, meta := range cp.Metas {
					if !newPresenceRefs[meta["presence_ref"].(string)] {
						leftPresences = append(leftPresences, cp)
					}
				}
			}

			if len(joinedPresences) > 0 {
				joins[key] = joinedPresences
			}

			if len(leftPresences) > 0 {
				leaves[key] = leftPresences
			}
		} else {
			joins[key] = []Presence{newPresences}
		}
	}

	return syncDiff(state, PresenceDiff{Joins: joins, Leaves: leaves}, onJoin, onLeave)
}

func syncDiff(
	state map[string][]Presence,
	diff PresenceDiff,
	onJoin func(key string, currentPresences, newPresences []Presence),
	onLeave func(key string, currentPresences, leftPresences []Presence),
) map[string][]Presence {
	for key, newPresences := range diff.Joins {
		currentPresences, ok := state[key]
		if !ok {
			currentPresences = []Presence{}
		}

		newPresenceRefs := make(map[string]bool)
		for _, np := range newPresences {
			newPresenceRefs[np.PresenceRef] = true
		}

		for _, cp := range currentPresences {
			for _, meta := range cp.Metas {
				if !newPresenceRefs[meta["presence_ref"].(string)] {
					newPresences = append(newPresences, Presence{PresenceRef: meta["presence_ref"].(string), Metas: []map[string]interface{}{meta}})
				}
			}
		}

		state[key] = newPresences
		onJoin(key, currentPresences, newPresences)
	}

	for key, leftPresences := range diff.Leaves {
		currentPresences, ok := state[key]
		if !ok {
			currentPresences = []Presence{}
		}

		leftPresenceRefs := make(map[string]bool)
		for _, lp := range leftPresences {
			leftPresenceRefs[lp.PresenceRef] = true
		}

		newCurrentPresences := []Presence{}
		for _, cp := range currentPresences {
			for _, meta := range cp.Metas {
				if !leftPresenceRefs[meta["presence_ref"].(string)] {
					newCurrentPresences = append(newCurrentPresences, cp)
				}
			}
		}

		if len(newCurrentPresences) == 0 {
			delete(state, key)
		} else {
			state[key] = newCurrentPresences
		}

		onLeave(key, currentPresences, leftPresences)
	}

	return state
}

func (rp *RealtimePresence) onJoin(callback func(key string, currentPresences, newPresences []Presence)) {
	rp.caller.OnJoin = callback
}

func (rp *RealtimePresence) onLeave(callback func(key string, currentPresences, leftPresences []Presence)) {
	rp.caller.OnLeave = callback
}

func (rp *RealtimePresence) onSync(callback func()) {
	rp.caller.OnSync = callback
}

func (rp *RealtimePresence) inPendingSyncState() bool {
	return rp.joinRef == "" || rp.joinRef != rp.channel.joinRef()
}

func cloneDeep(state map[string][]Presence) map[string][]Presence {
	newState := make(map[string][]Presence)
	for k, v := range state {
		newPresences := make([]Presence, len(v))
		copy(newPresences, v)
		newState[k] = newPresences
	}
	return newState
}

func transformState(state RawPresenceState) map[string][]Presence {
	newState := make(map[string][]Presence)
	for k, v := range state {
		newState[k] = []Presence{v}
	}
	return newState
}
