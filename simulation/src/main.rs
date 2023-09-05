use std::error::Error;
use std::fs::File;
use std::io::BufReader;
use std::path::Path;

use crate::models::data::JSONData;

pub mod models;

fn main() {
    let u: JSONData = read_data_from_file("../scripts/history.json").unwrap();
    // println!("{:#?}", u);

    u.simulate();
}

fn read_data_from_file<P: AsRef<Path>>(path: P) -> Result<JSONData, Box<dyn Error>> {
    // Open the file in read-only mode with buffer.
    let file = File::open(path)?;
    let reader = BufReader::new(file);

    // Read the JSON contents of the file as an instance of `User`.
    let u: JSONData = serde_json::from_reader(reader)?;

    // Return the `User`.
    Ok(u)
}
