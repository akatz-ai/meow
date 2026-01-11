package template

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/meow-stack/meow-machine/internal/types"
)

// Baker transforms template workflows into executable steps.
type Baker struct {
	// WorkflowID is the unique identifier for this workflow instance
	WorkflowID string

	// VarContext provides variable values for substitution
	VarContext *VarContext

	// Assignee is the default agent for agent steps
	Assignee string

	// Now allows injecting time for testing
	Now func() time.Time
}

// NewBaker creates a new Baker with default settings.
func NewBaker(workflowID string) *Baker {
	vc := NewVarContext()
	// Defer step output resolution to runtime - outputs aren't available at bake time
	vc.DeferStepOutputs = true
	return &Baker{
		WorkflowID: workflowID,
		VarContext: vc,
		Now:        time.Now,
	}
}

// BakeResult contains the steps generated from baking a workflow.
type BakeResult struct {
	Steps      []*types.Step // Steps for the workflow
	WorkflowID string        // Unique workflow instance ID
}

// BakeWorkflow transforms a workflow into types.Step objects.
func (b *Baker) BakeWorkflow(workflow *Workflow, vars map[string]string) (*BakeResult, error) {
	if workflow == nil {
		return nil, fmt.Errorf("workflow is nil")
	}

	// Apply provided variables to context
	// For file-type variables, read the file contents
	for k, v := range vars {
		if varDef, ok := workflow.Variables[k]; ok && varDef.Type == VarTypeFile {
			// Read file contents
			content, err := os.ReadFile(v)
			if err != nil {
				return nil, fmt.Errorf("reading file for variable %q: %w", k, err)
			}
			b.VarContext.Set(k, strings.TrimSpace(string(content)))
		} else {
			b.VarContext.Set(k, v)
		}
	}

	// Apply variable defaults from workflow
	for name, v := range workflow.Variables {
		if v.Default != nil && b.VarContext.Get(name) == "" {
			switch d := v.Default.(type) {
			case string:
				b.VarContext.Set(name, d)
			default:
				b.VarContext.Set(name, fmt.Sprintf("%v", d))
			}
		}
	}

	// Validate required variables (skip if deferring undefined variables, e.g., in foreach)
	if !b.VarContext.DeferUndefinedVariables {
		for name, v := range workflow.Variables {
			if v.Required && b.VarContext.Get(name) == "" {
				return nil, fmt.Errorf("required variable %q not provided", name)
			}
		}
	}

	// Set builtin variables
	b.VarContext.SetBuiltin("workflow_id", b.WorkflowID)

	// Process steps - create types.Step objects
	var steps []*types.Step
	for _, templateStep := range workflow.Steps {
		step, err := b.templateStepToStep(templateStep)
		if err != nil {
			return nil, fmt.Errorf("bake step %q: %w", templateStep.ID, err)
		}
		steps = append(steps, step)
	}

	return &BakeResult{
		Steps:      steps,
		WorkflowID: b.WorkflowID,
	}, nil
}

// templateStepToStep converts a template Step to a types.Step.
func (b *Baker) templateStepToStep(ts *Step) (*types.Step, error) {
	// Set step-specific builtins BEFORE substitution
	b.VarContext.SetBuiltin("step_id", ts.ID)

	// Determine executor type
	executor := b.determineExecutor(ts)

	// Create base step
	step := &types.Step{
		ID:       ts.ID,
		Executor: executor,
		Status:   types.StepStatusPending,
		Needs:    ts.Needs,
	}

	// Set executor-specific config
	if err := b.setStepConfig(step, ts); err != nil {
		return nil, err
	}

	return step, nil
}

// determineExecutor determines the executor type from template step fields.
func (b *Baker) determineExecutor(ts *Step) types.ExecutorType {
	// Executor field takes precedence
	if ts.Executor != "" {
		return types.ExecutorType(ts.Executor)
	}

	// Map Type field to executor
	switch ts.Type {
	case "task", "collaborative", "":
		return types.ExecutorAgent
	case "code":
		return types.ExecutorShell
	case "condition":
		return types.ExecutorBranch
	case "start":
		return types.ExecutorSpawn
	case "stop":
		return types.ExecutorKill
	case "expand":
		return types.ExecutorExpand
	case "gate":
		// Gates become branch with await-approval condition
		return types.ExecutorBranch
	default:
		return types.ExecutorAgent
	}
}

// setStepConfig sets the executor-specific configuration on the step.
func (b *Baker) setStepConfig(step *types.Step, ts *Step) error {
	switch step.Executor {
	case types.ExecutorShell:
		return b.setShellConfig(step, ts)
	case types.ExecutorSpawn:
		return b.setSpawnConfig(step, ts)
	case types.ExecutorKill:
		return b.setKillConfig(step, ts)
	case types.ExecutorExpand:
		return b.setExpandConfig(step, ts)
	case types.ExecutorBranch:
		return b.setBranchConfig(step, ts)
	case types.ExecutorForeach:
		return b.setForeachConfig(step, ts)
	case types.ExecutorAgent:
		return b.setAgentConfig(step, ts)
	default:
		return fmt.Errorf("unknown executor type: %s", step.Executor)
	}
}

// setShellConfig sets ShellConfig for shell executor steps.
func (b *Baker) setShellConfig(step *types.Step, ts *Step) error {
	// Get command from Command or Code field
	command := ts.Command
	if command == "" {
		command = ts.Code
	}

	// Substitute variables
	var err error
	command, err = b.VarContext.Substitute(command)
	if err != nil {
		return fmt.Errorf("substitute command: %w", err)
	}

	workdir := ts.Workdir
	if workdir != "" {
		workdir, err = b.VarContext.Substitute(workdir)
		if err != nil {
			return fmt.Errorf("substitute workdir: %w", err)
		}
	}

	// Substitute env values
	env := make(map[string]string)
	for k, v := range ts.Env {
		subV, err := b.VarContext.Substitute(v)
		if err != nil {
			return fmt.Errorf("substitute env %s: %w", k, err)
		}
		env[k] = subV
	}

	// Convert shell outputs to types format
	var outputs map[string]types.OutputSource
	if len(ts.ShellOutputs) > 0 {
		outputs = make(map[string]types.OutputSource)
		for k, v := range ts.ShellOutputs {
			outputs[k] = types.OutputSource{Source: v.Source}
		}
	}

	step.Shell = &types.ShellConfig{
		Command: command,
		Workdir: workdir,
		Env:     env,
		OnError: ts.OnError,
		Outputs: outputs,
	}
	return nil
}

// setSpawnConfig sets SpawnConfig for spawn executor steps.
func (b *Baker) setSpawnConfig(step *types.Step, ts *Step) error {
	// Get agent from Agent or Assignee field
	agent := ts.Agent
	if agent == "" {
		agent = ts.Assignee
	}

	var err error
	agent, err = b.VarContext.Substitute(agent)
	if err != nil {
		return fmt.Errorf("substitute agent: %w", err)
	}

	// Substitute adapter
	adapter := ts.Adapter
	if adapter != "" {
		adapter, err = b.VarContext.Substitute(adapter)
		if err != nil {
			return fmt.Errorf("substitute adapter: %w", err)
		}
	}

	workdir := ts.Workdir
	if workdir != "" {
		workdir, err = b.VarContext.Substitute(workdir)
		if err != nil {
			return fmt.Errorf("substitute workdir: %w", err)
		}
	}

	// Substitute env values
	env := make(map[string]string)
	for k, v := range ts.Env {
		subV, err := b.VarContext.Substitute(v)
		if err != nil {
			return fmt.Errorf("substitute env %s: %w", k, err)
		}
		env[k] = subV
	}

	// Substitute spawn_args
	spawnArgs := ts.SpawnArgs
	if spawnArgs != "" {
		spawnArgs, err = b.VarContext.Substitute(spawnArgs)
		if err != nil {
			return fmt.Errorf("substitute spawn_args: %w", err)
		}
	}

	step.Spawn = &types.SpawnConfig{
		Agent:         agent,
		Adapter:       adapter,
		Workdir:       workdir,
		Env:           env,
		ResumeSession: ts.ResumeSession,
		SpawnArgs:     spawnArgs,
	}
	return nil
}

// setKillConfig sets KillConfig for kill executor steps.
func (b *Baker) setKillConfig(step *types.Step, ts *Step) error {
	// Get agent from Agent or Assignee field
	agent := ts.Agent
	if agent == "" {
		agent = ts.Assignee
	}

	var err error
	agent, err = b.VarContext.Substitute(agent)
	if err != nil {
		return fmt.Errorf("substitute agent: %w", err)
	}

	graceful := true
	if ts.Graceful != nil {
		graceful = *ts.Graceful
	}

	timeout := 10 // default timeout in seconds

	step.Kill = &types.KillConfig{
		Agent:    agent,
		Graceful: graceful,
		Timeout:  timeout,
	}
	return nil
}

// setExpandConfig sets ExpandConfig for expand executor steps.
func (b *Baker) setExpandConfig(step *types.Step, ts *Step) error {
	template := ts.Template
	var err error
	template, err = b.VarContext.Substitute(template)
	if err != nil {
		return fmt.Errorf("substitute template: %w", err)
	}

	// Substitute variable values
	variables := make(map[string]string)
	for k, v := range ts.Variables {
		subV, err := b.VarContext.Substitute(v)
		if err != nil {
			return fmt.Errorf("substitute variable %s: %w", k, err)
		}
		variables[k] = subV
	}

	step.Expand = &types.ExpandConfig{
		Template:  template,
		Variables: variables,
	}
	return nil
}

// setForeachConfig sets ForeachConfig for foreach executor steps.
func (b *Baker) setForeachConfig(step *types.Step, ts *Step) error {
	// Handle items (expression) - apply variable substitution
	items := ts.Items
	var err error
	if items != "" {
		items, err = b.VarContext.Substitute(items)
		if err != nil {
			return fmt.Errorf("substitute items: %w", err)
		}
	}

	// Handle items_file - no variable substitution (it's a file path)
	itemsFile := ts.ItemsFile

	itemVar := ts.ItemVar
	indexVar := ts.IndexVar
	template := ts.Template

	template, err = b.VarContext.Substitute(template)
	if err != nil {
		return fmt.Errorf("substitute template: %w", err)
	}

	// Substitute variable values
	variables := make(map[string]string)
	for k, v := range ts.Variables {
		subV, err := b.VarContext.Substitute(v)
		if err != nil {
			return fmt.Errorf("substitute variable %s: %w", k, err)
		}
		variables[k] = subV
	}

	// Convert and substitute parallel (supports bool or string like "{{parallel}}")
	var parallel *bool
	switch v := ts.Parallel.(type) {
	case bool:
		parallel = &v
	case string:
		// Substitute variables first
		substituted, err := b.VarContext.Substitute(v)
		if err != nil {
			return fmt.Errorf("substitute parallel: %w", err)
		}
		// Parse the result as bool
		switch strings.ToLower(substituted) {
		case "true", "1", "yes":
			t := true
			parallel = &t
		case "false", "0", "no":
			f := false
			parallel = &f
		default:
			return fmt.Errorf("parallel must be true or false, got: %q", substituted)
		}
	}

	// Convert and substitute max_concurrent (supports int or string like "{{max_agents}}")
	var maxConcurrent string
	switch v := ts.MaxConcurrent.(type) {
	case string:
		maxConcurrent = v
	case int64:
		maxConcurrent = fmt.Sprintf("%d", v)
	case int:
		maxConcurrent = fmt.Sprintf("%d", v)
	case float64:
		maxConcurrent = fmt.Sprintf("%d", int(v))
	}
	if maxConcurrent != "" {
		maxConcurrent, err = b.VarContext.Substitute(maxConcurrent)
		if err != nil {
			return fmt.Errorf("substitute max_concurrent: %w", err)
		}
	}

	step.Foreach = &types.ForeachConfig{
		Items:         items,
		ItemsFile:     itemsFile,
		ItemVar:       itemVar,
		IndexVar:      indexVar,
		Template:      template,
		Variables:     variables,
		Parallel:      parallel,
		MaxConcurrent: maxConcurrent,
		Join:          ts.Join,
	}
	return nil
}

// setBranchConfig sets BranchConfig for branch executor steps.
func (b *Baker) setBranchConfig(step *types.Step, ts *Step) error {
	condition := ts.Condition
	var err error

	// Gate type uses await-approval condition
	if ts.Type == "gate" {
		condition = fmt.Sprintf("meow await-approval %s", step.ID)
	} else if condition != "" {
		condition, err = b.VarContext.Substitute(condition)
		if err != nil {
			return fmt.Errorf("substitute condition: %w", err)
		}
	}

	// Substitute workdir (shell-as-sugar support)
	workdir := ts.Workdir
	if workdir != "" {
		workdir, err = b.VarContext.Substitute(workdir)
		if err != nil {
			return fmt.Errorf("substitute workdir: %w", err)
		}
	}

	// Substitute env values (shell-as-sugar support)
	env := make(map[string]string)
	for k, v := range ts.Env {
		subV, err := b.VarContext.Substitute(v)
		if err != nil {
			return fmt.Errorf("substitute env %s: %w", k, err)
		}
		env[k] = subV
	}

	// Convert shell outputs to types format (shell-as-sugar support)
	var outputs map[string]types.OutputSource
	if len(ts.ShellOutputs) > 0 {
		outputs = make(map[string]types.OutputSource)
		for k, v := range ts.ShellOutputs {
			outputs[k] = types.OutputSource{Source: v.Source}
		}
	}

	step.Branch = &types.BranchConfig{
		Condition: condition,
		Timeout:   ts.Timeout,
		Workdir:   workdir,
		Env:       env,
		Outputs:   outputs,
		OnError:   ts.OnError,
	}

	// Convert expansion targets
	if ts.OnTrue != nil {
		step.Branch.OnTrue, err = b.expansionTargetToTypesBranch(ts.OnTrue)
		if err != nil {
			return fmt.Errorf("convert on_true: %w", err)
		}
	}
	if ts.OnFalse != nil {
		step.Branch.OnFalse, err = b.expansionTargetToTypesBranch(ts.OnFalse)
		if err != nil {
			return fmt.Errorf("convert on_false: %w", err)
		}
	}
	if ts.OnTimeout != nil {
		step.Branch.OnTimeout, err = b.expansionTargetToTypesBranch(ts.OnTimeout)
		if err != nil {
			return fmt.Errorf("convert on_timeout: %w", err)
		}
	}

	return nil
}

// expansionTargetToTypesBranch converts template ExpansionTarget to types.BranchTarget.
func (b *Baker) expansionTargetToTypesBranch(et *ExpansionTarget) (*types.BranchTarget, error) {
	if et == nil {
		return nil, nil
	}

	template := et.Template
	var err error
	if template != "" {
		template, err = b.VarContext.Substitute(template)
		if err != nil {
			return nil, fmt.Errorf("substitute template: %w", err)
		}
	}

	// Substitute variable values
	variables := make(map[string]string)
	for k, v := range et.Variables {
		subV, err := b.VarContext.Substitute(v)
		if err != nil {
			return nil, fmt.Errorf("substitute variable %s: %w", k, err)
		}
		variables[k] = subV
	}

	target := &types.BranchTarget{
		Template:  template,
		Variables: variables,
	}

	// Convert inline steps
	if len(et.Inline) > 0 {
		for _, inlineStep := range et.Inline {
			executor := types.ExecutorAgent
			if inlineStep.Executor != "" {
				executor = types.ExecutorType(inlineStep.Executor)
			} else {
				// Map Type field
				switch inlineStep.Type {
				case "task", "collaborative", "":
					executor = types.ExecutorAgent
				case "code":
					executor = types.ExecutorShell
				}
			}

			// Get prompt from Prompt or Instructions field
			prompt := inlineStep.Prompt
			if prompt == "" {
				prompt = inlineStep.Instructions
			}

			// Get agent from Agent or Assignee field
			agent := inlineStep.Agent
			if agent == "" {
				agent = inlineStep.Assignee
			}

			typesInlineStep := types.InlineStep{
				ID:       inlineStep.ID,
				Executor: executor,
				Prompt:   prompt,
				Agent:    agent,
				Needs:    inlineStep.Needs,
			}

			target.Inline = append(target.Inline, typesInlineStep)
		}
	}

	return target, nil
}

// setAgentConfig sets AgentConfig for agent executor steps.
func (b *Baker) setAgentConfig(step *types.Step, ts *Step) error {
	// Get agent from Agent or Assignee field, or baker default
	agent := ts.Agent
	if agent == "" {
		agent = ts.Assignee
	}
	if agent == "" {
		agent = b.Assignee
	}

	var err error
	if agent != "" {
		agent, err = b.VarContext.Substitute(agent)
		if err != nil {
			return fmt.Errorf("substitute agent: %w", err)
		}
	}

	// Get prompt from Prompt or Instructions field
	prompt := ts.Prompt
	if prompt == "" {
		prompt = ts.Instructions
	}
	if prompt != "" {
		prompt, err = b.VarContext.Substitute(prompt)
		if err != nil {
			return fmt.Errorf("substitute prompt: %w", err)
		}
	}

	mode := ts.Mode
	if mode == "" {
		mode = "autonomous"
	}

	// Convert outputs
	var outputs map[string]types.AgentOutputDef
	if len(ts.Outputs) > 0 {
		outputs = make(map[string]types.AgentOutputDef)
		for name, def := range ts.Outputs {
			outputs[name] = types.AgentOutputDef{
				Required:    def.Required,
				Type:        def.Type,
				Description: def.Description,
			}
		}
	}

	step.Agent = &types.AgentConfig{
		Agent:   agent,
		Prompt:  prompt,
		Mode:    mode,
		Outputs: outputs,
		Timeout: ts.Timeout,
	}
	return nil
}
