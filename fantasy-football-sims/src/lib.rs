mod league;
mod stats;
mod utils;

use league::LeagueData;
use stats::Stats;
use std::{str, vec};
use utils::set_panic_hook;
use wasm_bindgen::prelude::*;

#[macro_use]
extern crate prettytable;
use prettytable::Table;

// When the `wee_alloc` feature is enabled, use `wee_alloc` as the global
// allocator.
#[cfg(feature = "wee_alloc")]
#[global_allocator]
static ALLOC: wee_alloc::WeeAlloc = wee_alloc::WeeAlloc::INIT;

#[wasm_bindgen]
pub struct League {
    teams: Vec<String>,
    team_wins: Vec<i32>,
    sims_run: i32,
    n: i32,
    league_data: LeagueData,
    team_stats: Vec<Stats>,
}

#[wasm_bindgen]
impl League {
    pub fn new() -> League {
        set_panic_hook();

        let teams = vec![
            "Stroud Boys".to_string(),
            "Fully Chubbed".to_string(),
            "Tua these Nuts in ya mouth".to_string(),
            "Ceedeez nuts".to_string(),
        ];

        let bytes = include_bytes!("ff_data.json");

        let s = match str::from_utf8(bytes) {
            Ok(v) => v,
            Err(e) => panic!("Invalid UTF-8 sequence: {}", e),
        };

        let league_data: LeagueData = serde_json::from_str(s).unwrap();

        let n: i32 = 0;
        let sims_run: i32 = 100_000;
        let team_wins: Vec<i32> = vec![0, 0, 0, 0];
        let team_stats: Vec<Stats> = vec![
            Stats::new(vec![
                124.08, 167.09, 129.66, 174.56, 128.04, 154.58, 102.18, 140.36, 181.36, 166.2,
                126.9, 200.3, 196.62,
            ]),
            Stats::new(vec![
                146.92, 109.0, 195.06, 98.12, 104.78, 110.32, 97.16, 145.4, 143.54, 173.9, 139.54,
                133.98, 99.96,
            ]),
            Stats::new(vec![
                133.88, 150.66, 141.9, 101.28, 119.46, 142.42, 124.6, 139.86, 136.72, 123.76,
                134.58, 146.24, 127.8,
            ]),
            Stats::new(vec![
                152.2, 165.72, 160.64, 97.62, 123.38, 151.02, 187.76, 127.38, 154.18, 130.2,
                143.04, 166.16,
            ]),
        ];

        League {
            teams,
            team_wins,
            sims_run,
            n,
            league_data,
            team_stats,
        }
    }

    pub fn print(&mut self) -> String {
        if self.n < self.sims_run {
            self.sim();
        }

        let mut table1: Table = Table::new();
        table1.add_row(self.teams.clone().into());

        let mut r: Vec<String> = Vec::new();

        let rate1: f32 = (self.team_wins[0] as f32) / (self.n as f32);
        let rate2: f32 = (self.team_wins[1] as f32) / (self.n as f32);
        r.push(format!("{:.3}", rate1).to_string());
        r.push(format!("{:.3}", rate2).to_string());

        let rate3: f32 = (self.team_wins[2] as f32) / (self.n as f32);
        let rate4: f32 = (self.team_wins[3] as f32) / (self.n as f32);
        r.push(format!("{:.3}", rate3).to_string());
        r.push(format!("{:.3}", rate4).to_string());

        table1.add_row(r.into());
        table1.add_row(row![""]); // empty row
        table1.add_row(row!["N", &(format!("{:}", self.n).to_string())]);

        let mut table2: Table = Table::new();
        table2.add_row(row!["Team", "Average", "Std Deviation"]);

        for i in 0..4 {
            table2.add_row(row![
                format!("{:}", self.teams[i]),
                format!("{:.3}", self.team_stats[i].average()),
                format!("{:.3}", self.team_stats[i].std_deviation()),
            ]);
        }

        table1.set_titles(row!["Simulations"]);
        table2.set_titles(row!["Team Stats"]);

        return format!("{}\n{}", table1.to_string(), table2.to_string());
    }

    fn sim(&mut self) {
        for i in 0..2 {
            let r1: f64 = self.team_stats[2 * i].random_number();
            let r2: f64 = self.team_stats[(2 * i) + 1].random_number();

            if r1 > r2 {
                self.team_wins[2 * i] += 1;
            } else {
                self.team_wins[(2 * i) + 1] += 1;
            }
        }

        self.n += 1;
    }
}
