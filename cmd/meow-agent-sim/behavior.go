package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

// behaviorRegexCache caches compiled regular expressions.
var behaviorRegexCache = struct {
	sync.RWMutex
	cache map[string]*regexp.Regexp
}{
	cache: make(map[string]*regexp.Regexp),
}

// matchBehavior finds the first behavior that matches the prompt.
// Returns the matching behavior or the default behavior if no match.
func (s *Simulator) matchBehavior(prompt string) *Behavior {
	for i := range s.config.Behaviors {
		b := &s.config.Behaviors[i]
		if matches(b, prompt) {
			s.logger.Debug("behavior matched",
				"pattern", b.Match,
				"type", b.Type,
				"prompt", truncate(prompt, 50),
			)
			return b
		}
	}

	s.logger.Debug("using default behavior", "prompt", truncate(prompt, 50))
	return &s.config.Default.Behavior
}

// matches checks if a behavior pattern matches the prompt.
func matches(b *Behavior, prompt string) bool {
	matchType := b.Type
	if matchType == "" {
		matchType = "contains" // Default match type
	}

	switch matchType {
	case "regex":
		return matchRegex(b.Match, prompt)
	case "contains":
		return strings.Contains(prompt, b.Match)
	default:
		// Unknown match type, treat as contains
		return strings.Contains(prompt, b.Match)
	}
}

// matchRegex performs regex matching with caching.
func matchRegex(pattern, text string) bool {
	behaviorRegexCache.RLock()
	re, ok := behaviorRegexCache.cache[pattern]
	behaviorRegexCache.RUnlock()

	if !ok {
		var err error
		re, err = regexp.Compile(pattern)
		if err != nil {
			// Invalid regex, no match
			return false
		}
		behaviorRegexCache.Lock()
		behaviorRegexCache.cache[pattern] = re
		behaviorRegexCache.Unlock()
	}

	return re.MatchString(text)
}

// executeBehavior executes the action defined in a behavior.
func (s *Simulator) executeBehavior(b *Behavior, prompt string) error {
	action := b.Action

	s.logger.Debug("executing behavior",
		"action_type", action.Type,
		"delay", action.Delay,
		"pattern", b.Match,
	)

	// Apply delay (use default if not specified)
	delay := action.Delay
	if delay == 0 {
		delay = s.config.Timing.DefaultWorkDelay
	}
	if delay > 0 {
		time.Sleep(delay)
	}

	switch action.Type {
	case ActionComplete:
		return s.actionComplete(action)
	case ActionAsk:
		return s.actionAsk(action)
	case ActionFail:
		return s.actionFail(action)
	case ActionFailThenSucceed:
		return s.actionFailThenSucceed(b, action)
	case ActionHang:
		return s.actionHang()
	case ActionCrash:
		return s.actionCrash(action)
	default:
		// Unknown action type, default to complete
		s.logger.Warn("unknown action type, defaulting to complete", "type", action.Type)
		return s.actionComplete(action)
	}
}

// actionComplete signals successful completion via IPC.
func (s *Simulator) actionComplete(action Action) error {
	// Emit tool events if configured
	s.emitToolEvents(action.Events)

	// Print work output (simulating Claude's output)
	fmt.Println("Task completed successfully.")

	// Call meow done with outputs
	outputs := s.getOutputs(action)
	if outputs == nil {
		outputs = map[string]any{}
	}

	if err := s.ipc.StepDone(outputs); err != nil {
		s.logger.Error("meow done failed", "error", err)
		return err
	}

	s.transitionTo(StateIdle)
	return nil
}

// getOutputs returns the appropriate outputs for an action.
// If OutputsSequence is set, it returns the output for the current call count,
// repeating the last output after the sequence is exhausted.
func (s *Simulator) getOutputs(action Action) map[string]any {
	if len(action.OutputsSequence) == 0 {
		return action.Outputs
	}

	// Use a consistent key for tracking sequence position
	// We need to track this separately from attemptCounts which is used for fail_then_succeed
	// For now, use the current step ID as the key
	key := "sequence:" + s.stepID
	if s.sequenceCounts == nil {
		s.sequenceCounts = make(map[string]int)
	}

	idx := s.sequenceCounts[key]
	s.sequenceCounts[key]++

	if idx >= len(action.OutputsSequence) {
		// Repeat last output after sequence exhausted
		idx = len(action.OutputsSequence) - 1
	}

	s.logger.Debug("using outputs from sequence",
		"index", idx,
		"total", len(action.OutputsSequence),
	)

	return action.OutputsSequence[idx]
}

// actionAsk prints a question and transitions to ASKING state.
func (s *Simulator) actionAsk(action Action) error {
	question := action.Question
	if question == "" {
		question = "I have a question for you."
	}

	fmt.Println(question)
	s.transitionTo(StateAsking)
	return nil
}

// actionFail prints an error message without calling meow done.
func (s *Simulator) actionFail(action Action) error {
	message := action.FailMessage
	if message == "" {
		message = "An error occurred"
	}

	fmt.Fprintf(os.Stderr, "Error: %s\n", message)
	s.transitionTo(StateIdle)
	return nil
}

// actionFailThenSucceed fails N times, then succeeds.
func (s *Simulator) actionFailThenSucceed(b *Behavior, action Action) error {
	// Track attempts per pattern
	key := b.Match
	s.attemptCounts[key]++
	attempt := s.attemptCounts[key]

	failCount := action.FailCount
	if failCount == 0 {
		failCount = 1
	}

	if attempt <= failCount {
		// Fail this attempt
		s.logger.Debug("fail_then_succeed: failing",
			"attempt", attempt,
			"max_failures", failCount,
		)

		message := action.FailMessage
		if message == "" {
			message = fmt.Sprintf("Simulated failure (attempt %d/%d)", attempt, failCount)
		}

		fmt.Fprintf(os.Stderr, "Error: %s\n", message)
		s.transitionTo(StateIdle)
		return nil
	}

	// Success after failures
	s.logger.Debug("fail_then_succeed: succeeding",
		"attempt", attempt,
		"total_failures", failCount,
	)

	// Reset attempt counter for next time
	delete(s.attemptCounts, key)

	// Use success outputs if provided, otherwise use regular outputs
	return s.actionComplete(action)
}

// actionHang blocks forever (for testing timeout handling).
func (s *Simulator) actionHang() error {
	s.logger.Info("hanging forever (simulating stuck agent)")
	fmt.Println("Working on task...")

	// Block forever
	select {}
}

// actionCrash exits the process (for testing crash recovery).
func (s *Simulator) actionCrash(action Action) error {
	exitCode := action.ExitCode
	if exitCode == 0 {
		exitCode = 1 // Default to non-zero exit
	}

	s.logger.Info("crashing", "exit_code", exitCode)
	fmt.Fprintf(os.Stderr, "Simulated crash with exit code %d\n", exitCode)
	os.Exit(exitCode)

	return nil // Unreachable
}

// emitToolEvents emits tool events according to their timing.
// NOTE: Events should be listed in chronological order by "when" field.
// Events are emitted sequentially without sorting.
func (s *Simulator) emitToolEvents(events []EventDef) {
	if !s.config.Hooks.FireToolEvents || len(events) == 0 {
		return
	}

	startTime := time.Now()

	for _, event := range events {
		// Wait until it's time to emit this event
		targetTime := startTime.Add(event.When)
		waitDuration := time.Until(targetTime)
		if waitDuration > 0 {
			time.Sleep(waitDuration)
		}

		s.logger.Debug("emitting tool event",
			"type", event.Type,
			"data", event.Data,
			"when", event.When,
		)

		if err := s.ipc.Event(event.Type, event.Data); err != nil {
			s.logger.Warn("failed to emit event",
				"type", event.Type,
				"error", err,
			)
		}
	}
}
