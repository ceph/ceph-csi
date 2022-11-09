all

#Refer below url for more information about the markdown rules.
#https://github.com/markdownlint/markdownlint/blob/master/docs/RULES.md

rule 'MD013', :ignore_code_blocks => true, :tables => false, :line_length => 80

exclude_rule 'MD033' # In-line HTML: GitHub style markdown adds HTML tags
exclude_rule 'MD040' # Fenced code blocks should have a language specified
exclude_rule 'MD041' # First line in file should be a top level header
