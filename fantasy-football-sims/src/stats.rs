use js_sys::Math::{cos, log, random, sqrt};

pub struct Stats {
    average: f64,
    std_dev: f64,
    total: f64,
}

impl Stats {
    pub fn new(data: Vec<f64>) -> Stats {
        let total: f64 = data.clone().iter().sum::<f64>();
        let average: f64 = total / data.len() as f64;
        let std_dev: f64 = std_deviation(data);

        Stats {
            average,
            std_dev,
            total,
        }
    }

    // Thanks to SO for the answer on converting uniform distribution -> normal distr
    // https://stackoverflow.com/questions/25582882/javascript-math-random-normal-distribution-gaussian-bell-curve
    // using Box-Muller transform
    // Standard Normal variate using Box-Muller transform.
    // function gaussianRandom(mean=0, stdev=1) {
    //     const u = 1 - Math.random(); // Converting [0,1) to (0,1]
    //     const v = Math.random();
    //     const z = Math.sqrt( -2.0 * Math.log( u ) ) * Math.cos( 2.0 * Math.PI * v );
    //     // Transform to the desired mean and standard deviation:
    //     return z * stdev + mean;
    // }
    pub fn random_number(&self) -> f64 {
        let u: f64 = 1. - random();
        let v: f64 = random();
        let z: f64 = sqrt(-2. * log(u)) * cos(2.0 * 3.14159 * v);

        return z * self.std_dev + self.average;
    }

    pub fn average(&self) -> f64 {
        self.average
    }

    pub fn std_deviation(&self) -> f64 {
        self.std_dev
    }

    pub fn total(&self) -> f64 {
        self.total
    }
}

fn std_deviation(data: Vec<f64>) -> f64 {
    let count: usize = data.len();
    let data_mean: f64 = data.iter().sum::<f64>() as f64 / count as f64;

    let variance = data
        .iter()
        .map(|value| {
            let diff = data_mean - (*value as f64);

            diff * diff
        })
        .sum::<f64>()
        / count as f64;

    variance.sqrt()
}
