# Rule Engine

VaultSort uses a rule engine to decide which files to process and how.

## Evaluation Order

1. Rules are evaluated **in order** they appear in `config.toml`
2. **First matching rule wins** — subsequent rules are skipped
3. If no rule matches, the file is **left untouched**

## Matching Logic

All criteria within a rule must match (AND logic):

```
Match = Extension match
     AND StartsWith match
     AND EndsWith match
     AND CreatedAfter match
     AND ModifiedAfter match
     AND StartDir match (if set)
```

If a criterion is empty/not set, it's considered a pass (doesn't block the match).

### Extension Matching

- Case-insensitive
- Without the dot: `pdf` not `.pdf`
- Multiple extensions: `["jpg", "jpeg", "png"]`

### Prefix/Suffix Matching

- Case-sensitive
- Suffix is checked against the filename **before** the extension
- Example: `report_final.pdf` matches `endswith = "_final"`

### Time-based Matching

- Uses RFC3339 format: `"2024-01-15T10:30:00Z"`
- `created_after` uses file birthtime (macOS `Birthtimespec`)
- `modified_after` uses file modification time
- If birthtime isn't available, falls back to modification time

### StartDir Filter

- Only apply rule to files from watch paths whose **basename** matches
- Example: `start_dir = "Downloads"` only matches files from `~/Downloads`
- Comparison is case-insensitive

## Actions

When a rule matches, the action defines:

1. **Send file contents?** — If `send_file = true` and the file is text-based (< 1MB), up to 10KB of content is included in the LLM prompt
2. **Send filename?** — The original filename is always available via `{{filename}}`
3. **Prompt template** — Uses Go template-style placeholders:
   - `{{filename}}` — Original filename
   - `{{file_contents}}` — File content excerpt (if available)
   - `{{vault_dir}}` — The vault directory path

### Response Format

The LLM must respond with valid JSON:

```json
{"filename": "descriptive_name.pdf", "subdir": "documents/invoices/2024"}
```

- `filename`: New filename (with extension)
- `subdir`: Relative path under the vault directory

### Fallback

If the LLM call fails (network error, invalid response, etc.), the `fallback_subdir` is used and the original filename is preserved.

### EndDir Constraint

If `end_dir` is set, it's prepended to the LLM's suggested `subdir`. This is useful for constraining where files can go:

```toml
[rules.action]
end_dir = "documents"
# LLM response subdir "invoices/2024" → actual path: "documents/invoices/2024"
```

## Example Rules

### PDF Organization

```toml
[[rules]]
name = "pdfs"
enabled = true
[rules.match]
extension = ["pdf"]
[rules.action]
send_file = true
prompt = """Analyze this PDF and suggest a descriptive name and category.
File: {{filename}}
Respond in JSON: {"filename": "...", "subdir": "..."}"""
fallback_subdir = "unsorted/pdf"
```

### Image Categorization

```toml
[[rules]]
name = "photos"
enabled = true
[rules.match]
extension = ["jpg", "jpeg", "png", "heic"]
[rules.action]
send_file = false
send_filename = true
prompt = """Based on filename {{filename}}, suggest a photo category.
Respond in JSON: {"filename": "...", "subdir": "photos/YYYY/category"}"""
```

### Screenshot Cleanup

```toml
[[rules]]
name = "screenshots"
enabled = true
[rules.match]
startswith = "Screenshot"
extension = ["png"]
[rules.action]
start_dir = "Desktop"
prompt = """Filename: {{filename}}
Respond in JSON: {"filename": "screenshot.png", "subdir": "screenshots/2024"}"""
```
