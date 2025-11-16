---
agent: build
description: Review and refine the current change proposal and validate strictly.
---
The user has requested the following change proposal. Use the openspec instructions to create their change proposal.
<UserRequest>
  $ARGUMENTS
</UserRequest>
<!-- OPENSPEC:START -->
**Guardrails**
- Favor straightforward, minimal implementations first and add complexity only when it is requested or clearly required.
- Keep changes tightly scoped to the requested outcome.
- Refer to `openspec/AGENTS.md` (located inside the `openspec/` directory—run `ls openspec` or `openspec update` if you don't see it) if you need additional OpenSpec conventions or clarifications.
- Identify any vague or ambiguous details and ask the necessary follow-up questions before editing files.
- All proposal change updates MUST continue to implement the strict Test Driven Development (TDD) approach:
  - Ordered Phases
    - **RED Phase**: Write failing test(s) first
    - **GREEN Phase**: Write minimal code to make test(s) pass
    - **REFACTOR Phase**: Clean up code while keeping tests green
  - TDD Requirements
    - Write unit/integration tests BEFORE implementing the feature
    - Verify tests fail initially (RED)
    - Implement only enough code to make tests pass (GREEN)
    - Refactor as needed while maintaining >80% coverage
  - Tasks must be organized by feature area for clarity, but implementation MUST proceed test-first within each task.

**Sub Agent Requirements**
- Each subagent MUST look up and understand each passed file.
- Each subagent MUST review the proposal changes and check for edge cases, security issues, performance issues, error handling, best practices, etc.
- Each subagent must follow these report requirements in their responses
  - Be concise and direct, no verbose descriptions  and focus purely on technical instructions. Use bullet points and clear, actionable language.
  - Only include code blocks if the blocks are very short, otherwise simply refer to the relevant files and specific functions or logic that are being referenced.


**Steps**
1. Determine the proposal change id (aka `<id>`) from the current conversation or from the <UserRequest> section above.
1. Review `openspec/project.md`, run `openspec list` and `openspec list --specs`, and inspect the proposal changes files under `openspec/changes/<id>/` to ground the review of the proposal.
2. You MUST send all the files related to the proposed change to the subagent @architect-reviewer, the subagent @security-auditor, and any other subagents from the <UserRequest> section above. The subagents MUST adhere to the "Sub Agent Requirements" section above.
2. Only continue to the next step once all subagents respond.

2. 
3. Map the subagent recommendations updates to the appropriate change files
    - Update or add architectural reasoning in `design.md` if changed. ONLY include code blocks if it will help clarify details during implementation.
    - Update or add drafted spec deltas in `changes/<id>/specs/<capability>/spec.md` (one folder per capability) using `## ADDED|MODIFIED|REMOVED Requirements` with at least one `#### Scenario:` per requirement and cross-reference related capabilities when relevant.
    - Update or add drafted tasks in `tasks.md` as an ordered list of small, verifiable work items that deliver user-visible progress, include validation (tests, tooling), and highlight dependencies or parallelizable work.
7. Validate with `openspec validate <id> --strict` and resolve every issue before sharing the proposal.
8. Finally, you MUST compile all subagent recommendations and break each compiled items down by priority (include your recommendations if you have any, ask any clarifying questions if you have any)
    - Critical
    - High
    - Medium
    - Low


**Reference**
- Use `openspec show <id> --json --deltas-only` or `openspec show <spec> --type spec` to inspect details when validation fails.
- Search existing requirements with `rg -n "Requirement:|Scenario:" openspec/specs` before writing new ones.
- Explore the codebase with `rg <keyword>`, `ls`, or direct file reads so proposals align with current implementation realities.

ultrathink
<!-- OPENSPEC:END -->
