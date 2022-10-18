all

#Refer below url for more information about the markdown rules.
#https://github.com/markdownlint/markdownlint/blob/master/docs/RULES.md

rule 'MD013', :ignore_code_blocks => false, :tables => false, :line_length => 80

exclude_rule 'MD033' # In-line HTML: GitHub style markdown adds HTML tags
exclude_rule 'MD040' # Fenced code blocks should have a language specified
exclude_rule 'MD041' # First line in file should be a top level header
# TODO: Enable the rules after making required changes.
exclude_rule 'MD007' # Unordered list indentation
exclude_rule 'MD012' # Multiple consecutive blank lines
exclude_rule 'MD013' # Line length
exclude_rule 'MD047' # File should end with a single newline character