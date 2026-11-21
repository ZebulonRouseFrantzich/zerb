# Roadmap integrity checker

From the repository root, using Go 1.24 or newer:

```sh
go run ./tools/roadmapcheck
go run ./tools/roadmapcheck -root /path/to/checkout
go test ./tools/roadmapcheck
go test -race ./tools/roadmapcheck
go vet ./tools/roadmapcheck
go build -o /tmp/roadmapcheck ./tools/roadmapcheck
```

This standalone developer tool uses only the Go standard library. It reads
`docs/roadmap/**/*.md`, never changes documents, never fetches URLs, and never
executes the product or proof harnesses. It imports no product packages. No
Justfile or CI integration is included.

The checker assists the authority convention in `AGENTS.md`: PC requirements
belong to `product-contract.md`, ARCH invariants to `architecture.md`, and VAL
rows to `validation.md`. It checks document structure only. Passing it does not
qualify an implementation, prove a requirement, freeze a contract, satisfy any
qualification gate, or authorize a product phase. PC-001/PC-070, ARCH-029 and
VAL-001 continue to own the status and support boundaries.

## Checks and diagnostics

- Every concrete `PC-<digits>`, `ARCH-<digits>` and `VAL-<digits>` reference must
  have exactly one definition in its owning file. PC/ARCH definitions start an
  ATX or setext heading with the ID followed by whitespace or end of heading.
  VAL definitions occupy the complete first cell of a pipe table row, optionally
  in backticks. A heading or table in another file does not create authority.
- Duplicate owning definitions are reported at each definition. Missing IDs are
  reported once per source line. Inline code IDs are checked because roadmap
  prose commonly formats real references that way. Wildcards such as `PC-*`
  and IDs embedded in longer ASCII words or hyphenated names are ignored.
- Inline links and images, full/collapsed reference links, and link-reference
  definitions check target existence. All definitions are checked, including
  unused ones; repeated reference uses report a broken target at its definition.
  Undefined shortcuts such as `[ordinary prose]` remain ordinary text.
- Markdown fragments check ATX/setext heading anchors, including duplicate
  heading suffixes, or simple quoted `<a id="...">`/`<a name="...">` anchors.
  Heading slugs lowercase letters, retain Unicode letters/numbers and `_`/`-`,
  turn spaces/tabs into hyphens, and remove punctuation/formatting. Inline and
  defined reference link labels contribute their displayed text.
- Relative links resolve from their source document. Leading `/` means the
  selected repository root. Percent encoding, entities and Markdown punctuation
  escapes are decoded; queries do not affect local target existence. Empty
  fragments check existence only. External schemes and protocol-relative URLs
  are skipped without network access.
- Lexical escapes outside the root, symlink components, unavailable targets and
  special files are rejected. The explicitly selected root itself may be a
  symlink and is resolved once. Linked Markdown outside the roadmap is read for
  anchors only; its outbound links/IDs are outside this check's source scope.

Diagnostics sort by repository-relative path, physical line, and message.
Structural errors exit `1`; clean documents or warnings alone exit `0`;
invocation/root traversal failures exit `2`. `-h` exits `0`. Output ends with
error/warning counts and the reminder that this is a document-structure check.
With `go run`, Go may itself translate a nonzero program status into its own
status; use the built executable when an exact exit code is required.

`docs/roadmap/proof-results.md` is an append-only historical ledger, not current
authority. Its broken targets/anchors and missing-ID references are visible
warnings, not current-contract failures. Syntax errors remain errors. No other
file receives historical exemptions, and no phrase such as "example" or
"historical" suppresses arbitrary prose. Put illustrative broken links and
invented requirement IDs in code blocks. The checker does not edit the ledger.

## Deliberate Markdown limits

This is a documented subset for the repository's conventions, not a CommonMark
renderer. It supports single-line inline links/reference definitions, optional
quoted or parenthesized titles, balanced/escaped destination parentheses, angle
destinations, nested brackets in inline labels, same-line backtick code spans,
HTML comments, ordinary/quoted fenced blocks, simple list-item fenced blocks,
and lines indented by four spaces or a tab. Code blocks/comments are excluded
from both link and ID checks; inline code is excluded from links/HTML anchors.

Multiline links/titles/code spans/headings, complex nested lists, lazy blockquote
continuations, raw HTML blocks/links other than simple anchors, arbitrary HTML
`id` attributes, footnotes, custom heading attributes and renderer extensions
are outside this subset. Use single-line links and simple headings for checked
content. Recognized malformed inline links and unsupported reference-definition
forms produce syntax diagnostics; every unsupported construct cannot be
detected automatically. Four-space-indented list continuation prose is skipped
as code. Slug behavior for unusual Unicode symbols or complex inline markup
may differ from a hosting renderer. Non-Markdown fragments are diagnosed as
unsupported. Directory links check existence, without guessing an index file.
There is no live Git-revision link resolver, external URL checker, hostile
concurrent-filesystem guarantee, or validation of contract meaning/completeness.

## Fixtures and observed baseline

Fixtures cover the owner forms and duplicates, wrong owners, unresolved IDs,
local/encoded/root-relative links, images, references, titles/escapes, duplicate
and Unicode headings, simple HTML anchors, broken syntax, deterministic
diagnostics, historical warnings, code/comment boundaries, traversal/symlink
rejection and CLI exit classes. Tests copy their inputs into disposable roots.

On reviewed baseline `c048ce85ee2e8305f3a3a7af99db95a41b6eb324`, the checker
reports zero errors and two historical warnings in `proof-results.md`: line 66
links to the former PC-001 heading, and line 92 to the former PC-010 heading.
The document content is unchanged. These are structural observations only.
