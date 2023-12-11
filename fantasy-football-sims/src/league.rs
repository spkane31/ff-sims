use serde::{Deserialize, Serialize};
use std::{collections::HashMap, str};
use wasm_bindgen::prelude::*;

#[wasm_bindgen]
#[derive(Serialize, Deserialize, Clone)]
pub struct LeagueData {
    draft_data: Vec<DraftSelection>,
    matchup_data: HashMap<i32, Vec<Scoreboard>>,
}

#[wasm_bindgen]
impl LeagueData {
    pub fn new() -> LeagueData {
        LeagueData {
            draft_data: Vec::new(),
            matchup_data: HashMap::new(),
        }
    }

    pub fn draft_data(&self) -> Vec<DraftSelection> {
        self.draft_data.clone()
    }

    fn matchup_data(&self) -> HashMap<i32, Vec<Scoreboard>> {
        self.matchup_data.clone()
    }
}

#[wasm_bindgen]
#[derive(Serialize, Deserialize, Clone)]
pub struct Scoreboard {
    home_team: String,
    away_team: String,
    pub home_team_score: f32,
    pub away_team_score: f32,
}

#[wasm_bindgen]
impl Scoreboard {
    #[wasm_bindgen(getter)]
    pub fn home_team(&self) -> String {
        self.home_team.clone()
    }

    #[wasm_bindgen(setter)]
    pub fn set_home_team(&mut self, home_team: String) {
        self.home_team = home_team;
    }

    #[wasm_bindgen(getter)]
    pub fn away_team(&self) -> String {
        self.away_team.clone()
    }

    #[wasm_bindgen(setter)]
    pub fn set_away_team(&mut self, away_team: String) {
        self.away_team = away_team;
    }
}

#[wasm_bindgen]
#[derive(Serialize, Deserialize, Clone)]
pub struct DraftSelection {
    player_name: String,
}

#[wasm_bindgen]
impl DraftSelection {
    #[wasm_bindgen(getter)]
    pub fn player_name(&self) -> String {
        self.player_name.clone()
    }

    #[wasm_bindgen(setter)]
    pub fn set_player_name(&mut self, player_name: String) {
        self.player_name = player_name;
    }
}

// pub fn compute_team_statistics(data: LeagueData) -> HashMap<String, Vec<f32>> {
//     // let mut_data: LeagueData = data.into();
//     let weeks: usize = data.matchup_data().len();

//     let mut ret: HashMap<String, Vec<f32>> = HashMap::new();

//     let something = data.matchup_data().iter().map(|(_i, value)| {
//         value.iter().map(|scoreboard| {
//             if ret.contains_key(&scoreboard.home_team) {
//                 let scores = ret.get(&scoreboard.home_team).unwrap();
//                 scores.push(scoreboard.home_team_score);
//             }
//         })
//     });

//     // let team_scores: HashMap<String, Vec<f32>> = HashMap::new();

//     // for (_week, scoreboard) in mut_data.matchup_data.iter() {
//     //     for score in scoreboard.iter() {
//     //         if team_scores.contains_key(&score.home_team) {
//     //             // let v: &Vec<f32> = team_scores.get_mut(&score.home_team).unwrap();
//     //             // v.push(score.home_team_score);
//     //         }
//     //     }
//     // }

//     // ret.insert("Team 1".to_string(), Stats::new(vec![10.0, 12.0]));

//     ret
// }
