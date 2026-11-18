// Package docscheck holds the documentation gates of agx. It has no
// non-test code: run it with
//
//	go test -count=1 ./internal/docscheck/                       (task docs:check)
//	go test -count=1 ./internal/docscheck/ -run TestConfigSchemaFile -args -update
//	                                                             (task schema: regenerate schema/config.schema.json)
//	go test -count=1 ./internal/docscheck/ -run TestAgentBlockTable -args -update
//	                                                             (regenerate the agent-block tables of docs/config.md)
//
//	go test -count=1 ./internal/docscheck/ -run TestExamples        (task examples)
//	go test -count=1 ./internal/docscheck/ -run 'TestExamples/NAME$' -args -update
//	                                                             (write the real output into examples/NAME)
//	go test -count=1 ./internal/docscheck/ -run TestSnippets -args -update
//	                                                             (copy each snippet's region into its doc block)
//
// Files checked: README.md, CHANGELOG.md, CONTRIBUTING.md and SECURITY.md at
// the repo root, every .md under docs/ and .github/, and examples/README.md
// plus the README.md of each example folder (markdown deeper down, inside
// an example's home/, is fixture data). The content gates (local paths,
// private docs, attribution) also read every other file under examples/.
// Paths under testdata/, symlinks and gitignored files are skipped.
//
// The gates:
//
//   - Command reference drift (TestCommandReferenceDrift). The command tree
//     is built in-process from the parser's own declarations (cli.NewRoot,
//     cli.HelpTree: the data of agx help --json). Every visible command
//     needs a heading "agx <path>" in a file under docs/commands/, and its
//     section (up to the next heading of the same or a higher level) must
//     name every visible flag as inline code (`--flag`). The root's flags
//     go under a heading "Global flags". A table row starting with a flag
//     the command does not have, or a heading for a command agx does not
//     have, fails too.
//   - Runnable examples (TestRunnableExamples). The line
//     "<!-- runnable -->" directly above a ```sh block marks it runnable.
//     Each "$ agx …" line in the block is a command (leading NAME=value
//     words set its environment; '…' and "…" quote; a leading ~ is the
//     fixture home; pipes and substitutions are not supported). The lines
//     under it, up to the next "$ " line, are its shown output: what the
//     terminal shows, stderr notes first, then stdout. Comparison ignores
//     column alignment (every run of spaces is one space) and replaces run
//     ids, times, session stamps and agentio backup stamps
//     (.agentio-bak-20261118T061007Z) on both sides, so docs may show real
//     looking values; a line "..." matches any number of lines. Options go
//     in the marker as key=value words:
//     "<!-- runnable: fixture=empty dir=~/projects/client exit=125 -->".
//     fixture names the machine (default demo), dir the folder the commands
//     run in (default ~/projects/app), exit the last command's exit code
//     (default 0; earlier commands must exit 0). Commands of one block run
//     in order on one fresh fixture, so a block can set up and then show.
//   - Fixtures. "<!-- fixture: NAME -->" directly above a ```jsonc block
//     makes that block the config.jsonc of fixture NAME, so the config a
//     reader sees is the one the examples run against. The built-in
//     fixture "empty" has no config. Every fixture installs fake claude,
//     codex, gemini and pi binaries (they print what they received), a fake
//     gopass, the homes ~/.claude, ~/.claude-work, ~/.codex, ~/.gemini and
//     ~/.pi/agent, the folders ~/projects/app and ~/projects/client, and two
//     fake Claude conversations that ran in ~/projects/client (3f2a9c1e… in
//     ~/.claude, 7b1d4e20… in ~/.claude-work). In
//     shown output the fake binaries' folder reads /usr/local/bin and the
//     fixture home reads ~. Nothing touches a real account, binary,
//     network or quota.
//   - Example folders (TestExamples/NAME/SESSION). Each folder
//     examples/NAME (lowercase kebab-case) holds README.md and session.txt
//     (both required), optionally more sessions X.session.txt, a
//     config.jsonc (absent = no config) and a home/ folder copied over the
//     fixture home after the standard fixture is built (modes: executable
//     files 0755, tokens.jsonc 0600, the rest 0644, whatever the working
//     tree has, since git keeps only the executable bit); any other entry fails, and so does any entry of examples/
//     itself but README.md and the folders. Every session runs on its own fresh
//     fixture (the in-doc fixture machine, same fakes, homes and
//     conversations) with the binary built once per test process, all in
//     parallel. A session is a runnable block's body with four additions,
//     none of which agx ever prints (it does print "# " lines):
//     "[exit N]" alone on a line after a command's output is that
//     command's exit code (default 0 for every command); "#options
//     dir=~/x env=NAME=value" before the first command sets the folder
//     (default ~/projects/app) and an environment variable for every
//     command (env= repeats), and "mode=~/path=octal" sets one home/
//     file's mode (e.g. a 0644 tokens.jsonc that doctor fails); "#: text"
//     is a comment; region markers are
//     skipped. A trailing "< ~/file" or "< file" (no quotes or spaces) on
//     a command line is its stdin, read from the fixture home or the run
//     folder. With -args -update every command whose shown output or exit
//     code no longer matches gets the real ones (fixture paths shown as
//     above, indentation kept, its "..." lines lost); a comment or region
//     marker inside its output stays after the same number of output
//     lines. Tables are realigned from the columns of agx's real output
//     (a name wider than its column, "settings value", stays as agx
//     printed it). Matching commands, comments, options, markers and the
//     blank lines ending a block stay as written.
//   - Regions. "#region NAME" and "#endregion" (optionally "#endregion
//     NAME") mark a part of any example file; the marker may follow the
//     file's own comment token ("// #region x" in JSONC, "# #region x" in
//     an env file, "<!-- #region x -->"). Regions nest; a duplicate name,
//     a stray or mismatched #endregion and an unclosed region fail.
//     Marker lines belong to no region.
//   - Snippets (TestSnippets). "<!-- snippet: examples/NAME/FILE#REGION -->"
//     (or without #REGION for the whole file) directly above a fenced block
//     of any language makes the block a copy of that region: marker lines
//     excluded, trailing whitespace and trailing blank lines ignored. A
//     difference reads "file:line: snippet differs from
//     examples/x/session.txt#r (first difference at line N: doc "…" vs
//     file "…")" (<end> past the end of either side); a missing file or
//     region fails. With -args -update every differing block is rewritten
//     from its region instead (trailing whitespace and trailing blank lines
//     dropped, the fence's indentation added to each non-blank line); the
//     marker, the fences and matching blocks stay as written, and a
//     malformed snippet still fails. testdata/snippets.md proves the gate
//     on the seed examples (TestSnippetFixture).
//   - Config reference (TestConfigSchemaFile, TestConfigReference,
//     TestAgentBlockTable). schema/config.schema.json must equal
//     config.ConfigSchema (rewrite it with -update), the agent-block tables
//     of docs/config.md (between the "generated: agent-blocks" markers) must
//     equal the agents' typed block schemas (rewrite with -update), and
//     docs/config.md must name every key path of that
//     schema in inline code, with map keys as placeholders:
//     `profiles.<profile>.home`, `projects.<folder>.<agent>.login`. A key
//     table row naming a key the schema lacks fails.
//   - Links (TestLinks). Relative links and images must name an existing
//     file inside the repo; a #anchor into a markdown file must match a
//     heading slug (GitHub rules) or an explicit <a id>. External links are
//     not fetched; code is ignored.
//   - One physical line per paragraph, bullet or table cell
//     (TestNoHardWrap). Fenced code, indented code, tables, headings, HTML
//     blocks and blockquotes are exempt.
//   - No local machine paths, no mention of the private design docs and no
//     authoring attribution (TestNoLocalPaths, TestNoPrivateDocs,
//     TestNoAttribution). Naming the agents agx runs is fine.
//
// Each failure reads "file:line: reason" with the file relative to the
// repo root.
package docscheck
