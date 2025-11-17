---
description: Review the recent changes made in the current branch
agent: plan
subtask: false
---

<UserRequest>
  $ARGUMENTS
</UserRequest>

<!-- OPENSPEC:START -->

Changed files:
!`git diff main --name-only -- . ':!openspec' ':!.opencode' ':!AGENTS.md' ':!README.md'`


### Report Requirements
- Favor straightforward, minimal implementations first and add complexity only when it is requested or clearly required.
- Keep changes tightly scoped to the requested outcome.
- Be concise and direct, no verbose descriptions  and focus purely on technical instructions. Use bullet points and clear, actionable language.
- Only include code blocks if the blocks are very short, otherwise simply refer to the relevant files and specific functions that are being referenced.


The subagents MUST follow the report requirements above in their review report

**Steps**
1. Determine if the current conversation or the <UserRequest> section above contain a proposal change id (aka `<id>`).
2. Review `openspec/project.md`, run `openspec list` and `openspec list --specs`, and inspect the proposal changes files under `openspec/changes/<id>/` to ground the review of the branch changes against the current proposal.
3. You MUST send all the changed files above to the subagent @code-reviewer, the subagent @test-automator, and any other subagents from the <UserRequest> section above.
    - Each subagent MUST look up this branches changes for each file compared to the branch 'main'.
    - Each subagent MUST review the changes and check for edge cases, security issues, performance issues, error handling, best practices, testing quality and coverage, etc.
4. Only continue to the next step once all subagents respond.
5. Map the subagent recommendations updates to the appropriate change files
    - Update or add architectural reasoning in `design.md` if changed. ONLY include code blocks if it will help clarify details during implementation.
    - Update or add drafted spec deltas in `changes/<id>/specs/<capability>/spec.md` (one folder per capability) using `## ADDED|MODIFIED|REMOVED Requirements` with at least one `#### Scenario:` per requirement and cross-reference related capabilities when relevant.
    - Update or add drafted tasks in `tasks.md` as an ordered list of small, verifiable work items that deliver user-visible progress, include validation (tests, tooling), and highlight dependencies or parallelizable work.
6. Validate with `openspec validate <id> --strict` and resolve every issue before sharing the proposal.
7. Finally, you MUST compile all subagent recommendations and break each compiled items down by priority (include your recommendations if you have any, ask any clarifying questions if you have any)
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
