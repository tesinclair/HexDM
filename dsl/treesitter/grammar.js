/// <reference types="tree-sitter-cli/dsl" />
// @ts-check

const PREC = {
  sum: 1,
  product: 2,
  unary: 3,
};

/**
 * One or more `rule`, separated by `sep`.
 * @param {string} sep
 * @param {RuleOrLiteral} rule
 */
function sepBy1(sep, rule) {
  return seq(rule, repeat(seq(sep, rule)));
}

module.exports = grammar({
  name: 'hexcript',

  word: $ => $.identifier,

  extras: $ => [/\s/, $.comment],

  // In expression position a `{` may open a node body, a composite, or the
  // block of the enclosing `if`/`for`. GLR resolves each from right context.
  conflicts: $ => [
    [$.name, $.composite],
    [$.node],
  ],

  rules: {
    source_file: $ => repeat($._directive),

    _directive: $ => choice(
      $.node,
      $.binding,
      $.overlay,
      $.script,
    ),

    // ── Nodes: Widget, Container ─────────────────────────────────────
    node: $ => seq(
      field('head', $.node_head),
      '::',
      field('subtype', $.identifier),
      optional(field('id', $.id)),
      optional(field('arguments', $.argument_list)),
      optional(field('body', $.node_body)),
    ),

    node_head: $ => choice('Widget', 'Container'),

    id: $ => seq('<', field('name', $.identifier), '>'),

    node_body: $ => seq('{', repeat($.node), '}'),

    // ── Bindings: Auth ───────────────────────────────────────────────
    binding: $ => seq(
      field('head', $.binding_head),
      '::',
      field('subtype', $.identifier),
      field('arguments', $.argument_list),
    ),

    binding_head: $ => 'Auth',

    // ── Overlays: State, Trigger ─────────────────────────────────────
    overlay: $ => seq(
      field('head', $.overlay_head),
      '::',
      field('state', $.identifier),
      $.overlay_clause,
      repeat(seq(':', $.overlay_clause)),
    ),

    overlay_head: $ => choice('State', 'Trigger'),

    overlay_clause: $ => seq(
      '(',
      $.selector,
      ')',
      field('body', $.attribute_body),
    ),

    selector: $ => seq(
      optional(seq(field('source', $.identifier), '=>')),
      $.target_list,
    ),

    target_list: $ => sepBy1(',', $.target),

    target: $ => choice($.identifier, $.wildcard),

    wildcard: _ => '_',

    attribute_body: $ => seq('{', repeat($.attribute), '}'),

    // ── Scripts ──────────────────────────────────────────────────────
    script: $ => seq(
      field('head', $.script_head),
      '::',
      field('subtype', $.identifier),
      $.script_clause,
      repeat(seq(':', $.script_clause)),
    ),

    script_head: $ => 'Script',

    script_clause: $ => seq(
      '(',
      $.script_selector,
      ')',
      field('body', $.block),
    ),

    script_selector: $ => seq(
      optional(field('source', choice($.number, $.identifier))),
      '=>',
      $.scope_list,
    ),

    scope_list: $ => sepBy1(',', $.scope),

    scope: $ => seq(
      field('target', $.identifier),
      optional(seq('as', field('alias', $.identifier))),
    ),

    // ── Statements ───────────────────────────────────────────────────
    block: $ => seq('{', repeat($._statement), '}'),

    _statement: $ => choice(
      $.assignment,
      $.call_expression,
      $.if_statement,
      $.for_statement,
    ),

    assignment: $ => seq(
      field('left', $.name),
      '=',
      field('right', $._expression),
    ),

    if_statement: $ => seq(
      'if',
      field('condition', $._expression),
      field('consequence', $.block),
    ),

    for_statement: $ => seq(
      'for',
      field('variable', $.identifier),
      'in',
      field('range', $.range),
      field('body', $.block),
    ),

    range: $ => seq(
      field('start', $._expression),
      '..<',
      field('end', $._expression),
    ),

    // ── Expressions ──────────────────────────────────────────────────
    _expression: $ => choice(
      $.binary_expression,
      $.unary_expression,
      $._primary,
    ),

    binary_expression: $ => choice(
      prec.left(PREC.sum, seq(
        field('left', $._expression),
        field('operator', choice('+', '-')),
        field('right', $._expression),
      )),
      prec.left(PREC.product, seq(
        field('left', $._expression),
        field('operator', choice('*', '/')),
        field('right', $._expression),
      )),
    ),

    unary_expression: $ => prec(PREC.unary, seq(
      field('operator', '-'),
      field('operand', $._expression),
    )),

    _primary: $ => choice(
      $.node,
      $.call_expression,
      $.composite,
      $.name,
      $.number,
      $.string,
      $.boolean,
      $.parenthesized_expression,
    ),

    parenthesized_expression: $ => seq('(', $._expression, ')'),

    call_expression: $ => seq(
      field('function', $.name),
      field('arguments', $.argument_list),
    ),

    name: $ => seq(
      $.identifier,
      repeat(seq(choice('::', '.'), $.identifier)),
    ),

    composite: $ => seq(
      optional(field('tag', $.identifier)),
      '{',
      optional(sepBy1(',', $.composite_field)),
      '}',
    ),

    composite_field: $ => seq(
      optional(seq(field('name', $.identifier), '=')),
      field('value', $._expression),
    ),

    argument_list: $ => seq(
      '(',
      optional(sepBy1(',', $._argument)),
      ')',
    ),

    _argument: $ => choice($.attribute, $._expression),

    attribute: $ => seq(
      field('name', $.identifier),
      '=',
      field('value', $._expression),
    ),

    // ── Lexical ──────────────────────────────────────────────────────
    identifier: _ => /[A-Za-z_][A-Za-z0-9_]*/,

    number: _ => /[0-9]+(\.[0-9]+)?/,

    string: _ => token(seq('"', /[^"]*/, '"')),

    boolean: _ => choice('true', 'false'),

    comment: _ => token(seq('//', /[^\n]*/)),
  },
});
