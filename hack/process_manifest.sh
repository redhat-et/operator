#!/usr/bin/env bash

# Function to display help message
display_help() {
  echo "Usage: $0 <path_to_variable_file>"
  echo
  echo "This script replaces placeholders in the format <<VARIABLE_NAME>> with values"
  echo "from the specified variable file. The variable file must have entries in the format:"
  echo "  VARIABLE_NAME=value"
  echo
  exit 1
}

# Check if the variable file is provided
if [ -z "$1" ]; then
  echo "Error: Path to the variable file is not provided."
  display_help
fi

# Load the variable file
variable_file="$1"

# Check if the variable file exists and is readable
if [ ! -f "$variable_file" ] || [ ! -r "$variable_file" ]; then
  echo "Error: The specified variable file does not exist or is not readable."
  exit 1
fi


# Function to get the value of a variable from the file
get_variable_value() {
  local var_name="$1"
  grep -E "^${var_name}=" "$variable_file" | cut -d '=' -f 2-
}
# Function to process a single line and replace placeholders
process_line() {
  local line="$1"

  # Find all placeholders (in the format <<VAR_NAME>>)
  while [[ "$line" =~ \<\<([a-zA-Z_][a-zA-Z0-9_]*)\>\> ]]; do
    local placeholder="${BASH_REMATCH[0]}"
    local var_name="${BASH_REMATCH[1]}"
    local value=$(get_variable_value "$var_name")

    # If a value is found, replace the placeholder with the value
    if [ -n "$value" ]; then
      line="${line//$placeholder/$value}"
    else
      line="${line//$placeholder/}"
    fi
  done

  printf "%s\n" "$line"
}

while IFS= read -r line; do
    process_line "$line"
done
