package raft

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// NoopCommand is appended by every new leader so that entries from earlier
// terms can be committed safely. It is reserved and never accepted from clients.
const NoopCommand = "NOOP"

// Command is a parsed state-machine command.
type Command struct {
	Op    string
	Key   string
	Value string
}

// ParseCommand parses "SET key value", "SET key \"quoted value\"",
// "DELETE key" and the reserved "NOOP".
func ParseCommand(raw string) (Command, error) {
	raw = strings.TrimSpace(raw)
	if raw == NoopCommand {
		return Command{Op: NoopCommand}, nil
	}
	parts := strings.SplitN(raw, " ", 3)
	switch strings.ToUpper(parts[0]) {
	case "SET":
		if len(parts) != 3 || parts[1] == "" {
			return Command{}, fmt.Errorf("%w: SET requires a key and a value", ErrInvalidCommand)
		}
		value := parts[2]
		if strings.HasPrefix(value, `"`) {
			unquoted, err := strconv.Unquote(value)
			if err != nil {
				return Command{}, fmt.Errorf("%w: malformed quoted value", ErrInvalidCommand)
			}
			value = unquoted
		}
		return Command{Op: "SET", Key: parts[1], Value: value}, nil
	case "DELETE":
		if len(parts) != 2 || parts[1] == "" {
			return Command{}, fmt.Errorf("%w: DELETE requires exactly one key", ErrInvalidCommand)
		}
		return Command{Op: "DELETE", Key: parts[1]}, nil
	}
	return Command{}, fmt.Errorf("%w: unknown operation %q", ErrInvalidCommand, parts[0])
}

// ValidateClientCommand validates a command submitted by a client.
func ValidateClientCommand(raw string) error {
	cmd, err := ParseCommand(raw)
	if err != nil {
		return err
	}
	if cmd.Op == NoopCommand {
		return fmt.Errorf("%w: NOOP is reserved", ErrInvalidCommand)
	}
	return nil
}

// KVStore is the replicated state machine: a deterministic key-value map.
type KVStore struct {
	data map[string]string
}

func NewKVStore() *KVStore {
	return &KVStore{data: make(map[string]string)}
}

// Apply applies one committed entry. Invalid commands are ignored
// deterministically (they cannot be appended through Propose).
func (s *KVStore) Apply(e LogEntry) {
	cmd, err := ParseCommand(e.Command)
	if err != nil {
		return
	}
	switch cmd.Op {
	case "SET":
		s.data[cmd.Key] = cmd.Value
	case "DELETE":
		delete(s.data, cmd.Key)
	}
}

func (s *KVStore) Get(key string) (string, bool) {
	v, ok := s.data[key]
	return v, ok
}

func (s *KVStore) Len() int { return len(s.data) }

// Digest returns a short deterministic fingerprint of the current state.
func (s *KVStore) Digest() string {
	keys := make([]string, 0, len(s.data))
	for k := range s.data {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	h := sha256.New()
	for _, k := range keys {
		h.Write([]byte(k))
		h.Write([]byte{0})
		h.Write([]byte(s.data[k]))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}
