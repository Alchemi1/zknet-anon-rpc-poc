use std::fs;
use std::io::{BufRead, BufReader};

pub fn tail_file(path: &str, lines: usize) -> Result<Vec<String>, String> {
    let file = fs::File::open(path).map_err(|e| format!("cannot open {}: {}", path, e))?;
    let reader = BufReader::new(file);
    let all_lines: Vec<String> = reader.lines().filter_map(|l| l.ok()).collect();
    let start = if all_lines.len() > lines { all_lines.len() - lines } else { 0 };
    Ok(all_lines[start..].to_vec())
}
