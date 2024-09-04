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
* writing data to file and a database provided by `DATABASE_URL` environment vairable

Formerly:

* draft analysis
* season simulations
* playoff predictions

## Frontend

NextJS app for displaying data and allowing user interaction.

## Database

Go script for managing database. `gorm` does the database management for me, would like to expand this portion to do the data scraping as well. Would require reverse engineering the `espn_api` library. I use a CockroachDB serverless instance for the Postgres instance. Running `go run .` will build out the database.
