# Fantasy Football Report

A fun side-project where I run some simulations and do analysis for my fantasy football league.

This uses the [espn_api](https://github.com/cwendt/espn-api) Python library to gather data from ESPN's fantasy football site.

## Getting Started

To get started with the project:

1. Clone the repository with `git clone https://github.com/spkane31/ff-sims`
1. Set up your python environment:

```sh
# navigate to the scripts directory (where all python files are kept)
cd scripts
# Create a python virtual environment
python3.10 -m venv venv
# Install libraries
venv/bin/python -m  pip install -r requirements.txt
# Run the scripts!
venv/bin/python data.py
```

This will write data to a `history.json` file. The `history.json`  file acts as a cache, to re-fetch all data from ESPN simply delete the file.

## Scripts

* fetching data from ESPN
* draft analysis
* season simulations
* playoff predictions

## Simulation (work in progress)

I want to do the Monte Carlo simulation portion in Rust as a learning experience, maybe this year I actually will ...

## web-app

This will eventually be some sort of web page that I can easily host and easily share with friends.

* write the simulation results to a database (probably use a SQLite database that I can ship directly with the docker container)
* read from the database in the web-app
