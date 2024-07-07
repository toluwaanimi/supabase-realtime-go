package supabase_realtime_go

import (
	"encoding/json"
)

type Serializer struct{}

func NewSerializer() *Serializer {
	return &Serializer{}
}

func (s *Serializer) Decode(rawMessage []byte, callback func(msg RealtimeMessage)) {
	var msg RealtimeMessage
	err := json.Unmarshal(rawMessage, &msg)
	if err != nil {
		callback(RealtimeMessage{})
	} else {
		callback(msg)
	}
}

func (s *Serializer) Encode(payload interface{}, callback func(result string)) {
	result, err := json.Marshal(payload)
	if err != nil {
		callback("")
	} else {
		callback(string(result))
	}
}
