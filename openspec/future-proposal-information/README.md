# Future Proposal Information

This directory contains **planning documents** for future OpenSpec change proposals that have not yet been formalized.

## Purpose

When working on a change, you may identify related work that should be deferred to keep the current proposal focused. Rather than lose this valuable context, capture it here as a planning document.

## When to Use This Directory

Create a planning document here when:

- You've designed future work during a current proposal
- The feature is clearly out of scope for the current change
- You want to preserve context and design decisions
- The work depends on the current change being completed first

## What Goes Here

Each planning document should include:

- **Overview**: What problem does this solve?
- **User Stories**: How will users interact with this feature?
- **Design Decisions**: Technical approach and alternatives
- **Implementation Scope**: What will be built
- **Open Questions**: Things to validate before implementation
- **When to Create Formal Proposal**: Conditions for moving to `openspec/changes/`

## Directory Structure

```
future-proposal-information/
├── README.md                    # This file
├── git-remote-setup.md          # Future: Remote repository configuration
└── [other-future-feature].md    # Additional future proposals
```

## Lifecycle

1. **Create** planning document during current proposal work
2. **Reference** from current proposal's `design.md` or `proposal.md`
3. **Update** as you learn more from MVP feedback
4. **Promote** to formal `openspec/changes/[name]/` when ready to implement
5. **Archive** planning document once formal proposal is created (optional)

## Example Reference

From `openspec/changes/setup-git-repository/design.md`:

```markdown
## Future Work: Remote Repository Setup

See `openspec/future-proposal-information/git-remote-setup.md` for detailed planning.

This change focuses on local git initialization only. Remote setup deferred to post-MVP.
```

## Guidelines

### DO:
- ✅ Document detailed designs while context is fresh
- ✅ Include user stories and decision rationale
- ✅ Reference from active proposals
- ✅ Update based on user feedback
- ✅ Mark dependencies clearly

### DON'T:
- ❌ Create formal OpenSpec changes prematurely
- ❌ Duplicate content between planning doc and formal proposal
- ❌ Leave planning docs undiscoverable (always reference from related work)
- ❌ Let planning docs become stale (update or archive)

## Benefits

- **Preserves context**: Design decisions captured while fresh
- **Reduces scope creep**: Clear boundary between current and future work
- **Aids future implementers**: Detailed starting point when ready
- **Maintains focus**: Current proposals stay crisp and testable
- **Prevents rework**: Avoid rediscovering design decisions

## Related OpenSpec Conventions

This directory complements the standard OpenSpec workflow:

- **`openspec/specs/`** - Current truth (what IS built)
- **`openspec/changes/`** - Active proposals (what SHOULD change NOW)
- **`openspec/changes/archive/`** - Completed changes (historical record)
- **`openspec/future-proposal-information/`** - Planning (what MIGHT change LATER)

---

**Convention Established:** 2025-11-16  
**Status:** Active
