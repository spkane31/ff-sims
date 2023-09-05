use serde::{Deserialize, Serialize};

use super::data::Scoreboard;

#[derive(Serialize, Deserialize, Debug)]
pub struct TeamStanding {
    pub wins: i32,
    pub losses: i32,
    pub points_scored: f64,
    pub points_against: f64,
}

impl TeamStanding {
    pub fn add_game(mut self, scoreboard: Scoreboard) {
        if scoreboard.home_win() {
            self.wins += 1;
            self.points_scored += scoreboard.home_team_score.clone();
            self.points_against += scoreboard.away_team_score.clone();
        }
    }
}
