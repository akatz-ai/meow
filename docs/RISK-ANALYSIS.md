# MEOW Stack Risk Analysis

> Extracted from implementation planning. Covers risks, success criteria, and future considerations.

---

## Risk Analysis

### High Risk

1. **TOML Parsing Complexity**
   - Risk: Dynamic table names in TOML are tricky
   - Mitigation: Use `toml.Primitive` and two-phase parsing
   - Fallback: Pre-process TOML to standardize format

2. **Backward Compatibility**
   - Risk: Breaking existing templates
   - Mitigation: Format detection, legacy path
   - Fallback: Migration tool

### Medium Risk

1. **Circular References**
   - Risk: Infinite loops in template expansion
   - Mitigation: Cycle detection in loader
   - Fallback: Max expansion depth

2. **Variable Scoping**
   - Risk: Confusion about which variables are in scope
   - Mitigation: Explicit passing, no inheritance
   - Fallback: Debug logging

### Low Risk

1. **Label Conventions**
   - Risk: Inconsistent labeling
   - Mitigation: Constants, helper functions
   - Fallback: Migration/cleanup tool

2. **Performance**
   - Risk: Slow filtering with many beads
   - Mitigation: Index on labels
   - Fallback: Pagination, caching

---

## Success Criteria

### Functional

1. **Module Parsing**: Parse files with multiple `[workflow]` sections
2. **Local References**: `.workflow` references resolve correctly
3. **Wisp Detection**: Task beads automatically classified by tier
4. **Agent Visibility**: `meow prime` shows only wisp beads
5. **Workflow Progression**: Agents see step N/M progress
6. **Output Validation**: Required outputs enforced on close
7. **Crash Recovery**: `meow continue` resumes correctly

### Performance

1. **Parse Time**: Module file parses in <100ms
2. **Filter Time**: Tier filtering in <10ms for 1000 beads
3. **Memory**: No significant memory increase

### Usability

1. **Error Messages**: Include context, suggestions
2. **Documentation**: All new features documented
3. **Examples**: Templates demonstrate patterns

---

## Future Considerations

### Not In Scope (But Planned)

1. **Partial Workflow Execution**: `.workflow.step` syntax
2. **Template Inheritance**: `extends = "base-template"`
3. **Parallel Execution**: Multiple agents on same workflow
4. **Remote Templates**: `https://...` references

### Open Questions

1. Should modules support shared variables at module level?
2. Should `internal` be the default for helper workflows?
3. What's the max expansion depth before error?
