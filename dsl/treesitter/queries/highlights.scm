; Later patterns override earlier ones on the same node, so the general
; cases come first and the specific ones last.

; ── Names (general case) ─────────────────────────────────────────────
(name (identifier) @variable)

; Daemon::User, Daemon::PasswordLength — namespaced value
(name
  (identifier) @module
  "::"
  (identifier) @constant)

; title.color, pw.offset_x — field access
(name
  (identifier) @variable
  "."
  (identifier) @property)

; foo(...) — unqualified call
(call_expression
  function: (name . (identifier) @function.call .))

; Daemon::GetTime(), Math::Mod(), Util::HSVToHex()
(call_expression
  function: (name
              (identifier) @module
              "::"
              (identifier) @function.call))

; ── Directive heads ──────────────────────────────────────────────────
(node_head) @keyword
(binding_head) @keyword
(overlay_head) @keyword
(script_head) @keyword

; Subtype after `::` — Text, VBox, Path, Fprint, CapsLock, Frame, ...
(node subtype: (identifier) @type)
(binding subtype: (identifier) @type)
(script subtype: (identifier) @type)
(overlay state: (identifier) @type)

; ── Ids and selectors ────────────────────────────────────────────────
(id name: (identifier) @label)
(id ["<" ">"] @punctuation.bracket)

(selector source: (identifier) @variable)
(target (identifier) @variable)
(wildcard) @constant.builtin

(script_selector source: (identifier) @variable)
(scope target: (identifier) @variable)
(scope alias: (identifier) @variable.parameter)

(for_statement variable: (identifier) @variable.parameter)

; ── Attributes and composite fields ──────────────────────────────────
(attribute name: (identifier) @property)
(composite_field name: (identifier) @property)
(composite tag: (identifier) @type)

; ── Keywords ─────────────────────────────────────────────────────────
"if" @keyword.conditional
"for" @keyword.repeat
["in" "as"] @keyword.operator

; ── Literals ─────────────────────────────────────────────────────────
(string) @string
(number) @number
(boolean) @boolean
(comment) @comment @spell

; ── Operators and punctuation ────────────────────────────────────────
[
  "=>"
  "..<"
  "+"
  "-"
  "*"
  "/"
  "="
] @operator

[
  "::"
  "."
  ","
  ":"
] @punctuation.delimiter

[
  "("
  ")"
  "{"
  "}"
] @punctuation.bracket
