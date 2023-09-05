use serde::{Deserialize, Serialize};

use std::collections::{HashMap, HashSet};
use std::vec::Vec;

#[derive(Serialize, Deserialize, Debug)]
pub struct JSONData {
    pub matchup_data: HashMap<String, Vec<Scoreboard>>,
    pub draft_data: Vec<DraftPick>,
    pub activity_data: Vec<Activity>,

    #[serde(skip)]
    teams: HashSet<String>,
    current_standings: HashMap<String, TeamStanding>,
}

impl JSONData {
    pub fn simulate(mut self) {
        println!("Simulating!");
        self.set_teams();

        // self.get_current_standings();
    }

    fn set_teams(mut self) {
        for (_week, scoreboards) in self.matchup_data.iter() {
            for scoreboard in scoreboards.iter() {
                self.teams.insert(scoreboard.away_team.clone());
                self.teams.insert(scoreboard.home_team.clone());
            }
            println!(
                "Found {:} teams in league ({:?})",
                self.teams.len(),
                self.teams
            );
            return;
        }
    }

    // fn get_current_standings(mut self) {
    //     for (_week, scoreboards) in self.matchup_data.iter() {
    //         for scoreboard in scoreboards.iter() {
    //             if scoreboard.home_team_score != 0.0 && scoreboard.away_team_projected_score != 0.0
    //             {
    //                 return;
    //             }
    //         }
    //     }
    // }
}

#[derive(Serialize, Deserialize, Debug)]
pub struct Scoreboard {
    pub home_team: String,
    pub home_team_score: f64,
    pub home_team_projected_score: f64,
    pub home_lineup: Vec<Lineup>,
    pub away_team: String,
    pub away_team_score: f64,
    pub away_team_projected_score: f64,
    pub away_lineup: Vec<Lineup>,
}

impl Scoreboard {
    pub fn home_win(self) -> bool {
        return self.home_team_score > self.away_team_score;
    }
}

#[derive(Serialize, Deserialize, Debug)]
pub struct Lineup {
    name: String,
    projection: f64,
    actual: f64,
    position: String,
    status: String,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct DraftPick {
    player_name: String,
    player_id: i32,
    team: i32,
    team_name: String,
    total_points: f64,
    round_number: i32,
    round_pick: i32,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct Activity {
    date: i64,
    actions: Vec<Action>,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct Action {
    team: String,
    action: String,
    player: Player,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct Player {
    name: String,
    player_id: i64,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct TeamStanding {
    pub wins: i32,
    pub losses: i32,
    pub points_scored: f32,
    pub points_against: f32,
}
