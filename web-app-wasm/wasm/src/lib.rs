use wasm_bindgen::prelude::*;

#[wasm_bindgen]
pub fn add(x: i32, y: i32) -> i32 {
    x + y
}

#[cfg(test)]
mod tests {
    use super::add;

    #[test]
    fn _add() {
        assert_eq!(add(2, 2), 4);
    }
}

#[macro_use]
extern crate prettytable;
use prettytable::Table;

#[wasm_bindgen]
pub fn simulate() -> String {
    // let mut table1: Table = Table::new();
    // return format!("{}", table1.to_string());
    "".to_string()
}

#[wasm_bindgen]
pub fn all_time_leaders_table() -> String {
    let mut table: Table = Table::new();
    table.add_row(row!["", "Wins", "Losses", "Points For", "Points Against"]);
    table.set_titles(row!["All Time Leaderboards"]);
    return format!("{}", table.to_string());
}

// #[wasm_bindgen]
// impl League {
//     pub fn new() -> League {
//         set_panic_hook();

//         let teams = vec![
//             "Daddy Doepker".to_string(),
//             "Josh's Jaqs".to_string(),
//             "Ceedeez nuts".to_string(),
//             "The Stroud Boys".to_string(),
//             "Walker Texas Nutter".to_string(),
//             "Fully Chubbed".to_string(),
//         ];

//         // let bytes = include_bytes!("ff_data.json");

//         // let s = match str::from_utf8(bytes) {
//         //     Ok(v) => v,
//         //     Err(e) => panic!("Invalid UTF-8 sequence: {}", e),
//         // };

//         // let league_data: LeagueData = serde_json::from_str(s).unwrap();

//         let n: i32 = 0;
//         let sims_run: i32 = 100_000;
//         // let team_wins: Vec<i32> = vec![0, 0, 0, 0];
//         let team_stats: Vec<Stats> = vec![
//             // First place: Josh
//             Stats::new(vec![
//                 125.34, 126.26, 135.94, 184.38, 177.76, 119.94, 135.32, 180.64, 125.78, 114.82,
//                 132.06, 198.86, 165.64, 141.64,
//             ]),
//             // Second place: Ethan
//             Stats::new(vec![
//                 101.6, 148.24, 152.4, 182.64, 100.1, 152.68, 150.88, 131.64, 96.9, 112.3, 111.66,
//                 161.74, 136.56, 187.46,
//             ]),
//             // Third place: Burns
//             Stats::new(vec![
//                 152.2, 165.72, 160.64, 97.62, 123.38, 151.02, 187.76, 187.52, 127.38, 154.18,
//                 130.2, 143.04, 166.16, 113.0,
//             ]),
//             // Fourth place: Sean
//             Stats::new(vec![
//                 124.08, 167.09, 129.66, 174.56, 128.04, 154.58, 102.18, 140.36, 181.36, 166.2,
//                 126.9, 200.3, 196.62, 132.66,
//             ]),
//             // Fifth place: Jack
//             Stats::new(vec![
//                 110.56, 174.88, 175.02, 93.5, 111.26, 115.34, 145.8, 130.08, 106.5, 127.72, 125.8,
//                 140.78, 160.0, 123.9,
//             ]),
//             // Sixth place: Toth
//             Stats::new(vec![
//                 146.92, 109.0, 195.06, 98.12, 104.78, 110.32, 97.16, 145.4, 143.54, 173.9, 139.54,
//                 133.98, 99.96, 143.32,
//             ]),
//         ];

//         let final_standings: [[u32; 6]; 6] = [
//             [0, 0, 0, 0, 0, 0],
//             [0, 0, 0, 0, 0, 0],
//             [0, 0, 0, 0, 0, 0],
//             [0, 0, 0, 0, 0, 0],
//             [0, 0, 0, 0, 0, 0],
//             [0, 0, 0, 0, 0, 0],
//         ];

//         League {
//             teams,
//             // team_wins,
//             sims_run,
//             n,
//             // league_data,
//             team_stats,
//             final_standings,
//         }
//     }

//     pub fn print(&mut self) -> String {
//         if self.n < self.sims_run {
//             self.sim();
//         }

//         let mut table1: Table = Table::new();
//         let mut teams_header: Vec<String> = Vec::new();
//         teams_header.push("".to_string());
//         for team in self.teams.clone().iter() {
//             teams_header.push(team.to_string());
//         }
//         table1.add_row(teams_header.into());

//         for team_idx in 0..6 {
//             let mut r: Vec<String> = Vec::new();
//             r.push(format!("{:}", team_idx + 1));
//             for pos_idx in 0..6 {
//                 let val: f64 = self.final_standings[pos_idx][team_idx] as f64;
//                 if val == 0.0 {
//                     r.push(format!(""))
//                 } else {
//                     r.push(format!("{:.1} %", 100.0 * val / self.n as f64))
//                 }
//             }
//             table1.add_row(r.into());
//         }

//         table1.add_row(row![""]); // empty row
//         table1.add_row(row!["N", &(format!("{:}", self.n).to_string())]);

//         let mut table2: Table = Table::new();
//         table2.add_row(row!["Team", "Average", "Std Deviation", "Total"]);

//         for i in 0..self.teams.len() {
//             table2.add_row(row![
//                 format!("{:}", self.teams[i]),
//                 format!("{:.2}", self.team_stats[i].average()),
//                 format!("{:.2}", self.team_stats[i].std_deviation()),
//                 format!("{:.2}", self.team_stats[i].total()),
//             ]);
//         }

//         table1.set_titles(row!["Final Results"]);
//         table2.set_titles(row!["Team Stats"]);

//         return format!("{}\n{}", table1.to_string(), table2.to_string());
//     }

//     pub fn run_simulations(&mut self, n: u64) {
//         for _ in 0..n {
//             self.sim();
//         }
//     }

//     fn sim(&mut self) {
//         if self.n > i32::MAX || self.n > 800_000 {
//             return;
//         }

//         // Winners for round 1: 4 vs 5
//         let winner_4_v_5: i32 =
//             if self.team_stats[3].random_number() > self.team_stats[4].random_number() {
//                 3
//             } else {
//                 4
//             };
//         let loser_4_v_5: i32 = if winner_4_v_5 == 3 { 4 } else { 3 };

//         // Winners for round 1: 3 vs 6
//         let winner_3_v_6: i32 =
//             if self.team_stats[2].random_number() > self.team_stats[5].random_number() {
//                 2
//             } else {
//                 5
//             };
//         let loser_3_v_6: i32 = if winner_3_v_6 == 2 { 5 } else { 2 };

//         // Consolation game
//         if self.team_stats[loser_4_v_5 as usize].random_number()
//             > self.team_stats[loser_3_v_6 as usize].random_number()
//         {
//             self.final_standings[loser_4_v_5 as usize][4] += 1;
//             self.final_standings[loser_3_v_6 as usize][5] += 1;
//         } else {
//             self.final_standings[loser_4_v_5 as usize][5] += 1;
//             self.final_standings[loser_3_v_6 as usize][4] += 1;
//         }

//         // Semi final 1 vs 5/6
//         let first_semi_winner: i32 = if self.team_stats[0].random_number()
//             > self.team_stats[winner_4_v_5 as usize].random_number()
//         {
//             0
//         } else {
//             winner_4_v_5
//         };
//         let first_semi_loser: i32 = if first_semi_winner == 0 {
//             winner_4_v_5
//         } else {
//             0
//         };

//         // Semi final 2: 2 vs 3/4
//         let second_semi_winner: i32 = if self.team_stats[1].random_number()
//             > self.team_stats[winner_3_v_6 as usize].random_number()
//         {
//             1
//         } else {
//             winner_3_v_6
//         };
//         let second_semi_loser: i32 = if second_semi_winner == 1 {
//             winner_3_v_6
//         } else {
//             1
//         };

//         // Championship game
//         if self.team_stats[first_semi_winner as usize].random_number()
//             > self.team_stats[second_semi_winner as usize].random_number()
//         {
//             self.final_standings[first_semi_winner as usize][0] += 1;
//             self.final_standings[second_semi_winner as usize][1] += 1;
//         } else {
//             self.final_standings[first_semi_winner as usize][1] += 1;
//             self.final_standings[second_semi_winner as usize][0] += 1;
//         }

//         // 3rd place game
//         if self.team_stats[first_semi_loser as usize].random_number()
//             > self.team_stats[second_semi_loser as usize].random_number()
//         {
//             self.final_standings[first_semi_loser as usize][2] += 1;
//             self.final_standings[second_semi_loser as usize][3] += 1;
//         } else {
//             self.final_standings[first_semi_loser as usize][3] += 1;
//             self.final_standings[second_semi_loser as usize][2] += 1;
//         }

//         self.n += 1;
//     }
// }
