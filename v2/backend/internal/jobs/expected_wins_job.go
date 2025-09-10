package jobs

import (
	"backend/internal/database"
	"backend/internal/models"
	"backend/internal/simulation"
	"log"
	"time"
)

// ExpectedWinsJob handles automated expected wins calculations
type ExpectedWinsJob struct {
	LeagueID uint
	Year     uint
	Week     uint
}

// WeeklyExpectedWinsJob runs after each week's games complete
func WeeklyExpectedWinsJob() {
	log.Printf("Starting weekly expected wins job at %v", time.Now())

	leagues, err := getAllLeagues()
	if err != nil {
		log.Printf("Failed to get leagues: %v", err)
		return
	}

	currentYear := uint(time.Now().Year())

	for _, league := range leagues {
		processLeagueWeeklyExpectedWins(league, currentYear)
	}

	log.Printf("Completed weekly expected wins job at %v", time.Now())
}

// processLeagueWeeklyExpectedWins processes expected wins for a single league
func processLeagueWeeklyExpectedWins(league models.League, currentYear uint) {
	db := database.DB

	// Find the most recent completed week
	lastCompletedWeek, err := models.GetLastCompletedWeek(db, league.ID, currentYear)
	if err != nil {
		log.Printf("Failed to get last completed week for league %d: %v", league.ID, err)
		return
	}

	if lastCompletedWeek == 0 {
		log.Printf("No completed weeks found for league %d, year %d", league.ID, currentYear)
		return
	}

	// Check if we've already processed this week
	processed, err := models.IsWeekProcessed(db, league.ID, currentYear, lastCompletedWeek)
	if err != nil {
		log.Printf("Failed to check if week is processed for league %d: %v", league.ID, err)
		return
	}

	if processed {
		log.Printf("Week %d already processed for league %d, year %d", lastCompletedWeek, league.ID, currentYear)
		return
	}

	// Process the week
	log.Printf("Processing week %d for league %d, year %d", lastCompletedWeek, league.ID, currentYear)
	err = simulation.ProcessWeeklyExpectedWins(league.ID, currentYear, lastCompletedWeek)
	if err != nil {
		log.Printf("Failed to process weekly expected wins for league %d, week %d: %v",
			league.ID, lastCompletedWeek, err)
		return
	}

	log.Printf("Successfully processed week %d for league %d", lastCompletedWeek, league.ID)

	// Check if this was the final regular season week
	if simulation.IsRegularSeasonComplete(db, league.ID, currentYear) {
		log.Printf("Regular season complete for league %d, finalizing season expected wins", league.ID)
		err = simulation.FinalizeSeasonExpectedWins(league.ID, currentYear)
		if err != nil {
			log.Printf("Failed to finalize season expected wins for league %d: %v", league.ID, err)
			return
		}
		log.Printf("Successfully finalized season expected wins for league %d", league.ID)
	}
}

// ScheduledMaintenanceJob performs routine maintenance on expected wins data
func ScheduledMaintenanceJob() {
	log.Printf("Starting expected wins maintenance job at %v", time.Now())

	// Clean up old calculation data older than 5 years
	cutoffDate := time.Now().AddDate(-5, 0, 0)
	cutoffYear := uint(cutoffDate.Year())

	db := database.DB

	// Clean old weekly data
	result := db.Where("year < ?", cutoffYear).Delete(&models.WeeklyExpectedWins{})
	if result.Error != nil {
		log.Printf("Failed to clean old weekly expected wins data: %v", result.Error)
	} else if result.RowsAffected > 0 {
		log.Printf("Cleaned %d old weekly expected wins records", result.RowsAffected)
	}

	// Clean old season data (though we probably want to keep this longer)
	// For now, only clean data older than 10 years
	oldCutoffYear := uint(time.Now().AddDate(-10, 0, 0).Year())
	result = db.Where("year < ?", oldCutoffYear).Delete(&models.SeasonExpectedWins{})
	if result.Error != nil {
		log.Printf("Failed to clean old season expected wins data: %v", result.Error)
	} else if result.RowsAffected > 0 {
		log.Printf("Cleaned %d old season expected wins records", result.RowsAffected)
	}

	log.Printf("Completed expected wins maintenance job at %v", time.Now())
}

// Helper functions

// getAllLeagues returns all leagues from the database
func getAllLeagues() ([]models.League, error) {
	db := database.DB
	var leagues []models.League
	err := db.Find(&leagues).Error
	return leagues, err
}

// ExpectedWinsJobScheduler sets up recurring jobs for expected wins calculations
type ExpectedWinsJobScheduler struct {
	weeklyTicker      *time.Ticker
	maintenanceTicker *time.Ticker
	stopChan          chan bool
}

// NewExpectedWinsJobScheduler creates a new job scheduler
func NewExpectedWinsJobScheduler() *ExpectedWinsJobScheduler {
	return &ExpectedWinsJobScheduler{
		stopChan: make(chan bool),
	}
}

// Start begins the scheduled job execution
func (s *ExpectedWinsJobScheduler) Start() {
	log.Println("Starting expected wins job scheduler")

	// Run weekly job every hour during the season (can be adjusted)
	s.weeklyTicker = time.NewTicker(1 * time.Hour)

	// Run maintenance job daily at 3 AM (adjust as needed)
	s.maintenanceTicker = time.NewTicker(24 * time.Hour)

	go func() {
		for {
			select {
			case <-s.weeklyTicker.C:
				// Only run during football season (September through January)
				now := time.Now()
				month := now.Month()
				if month >= time.September || month <= time.January {
					WeeklyExpectedWinsJob()
				}

			case <-s.maintenanceTicker.C:
				// Check if it's around 3 AM
				now := time.Now()
				if now.Hour() == 3 {
					ScheduledMaintenanceJob()
				}

			case <-s.stopChan:
				log.Println("Stopping expected wins job scheduler")
				return
			}
		}
	}()
}

// Stop halts the scheduled job execution
func (s *ExpectedWinsJobScheduler) Stop() {
	if s.weeklyTicker != nil {
		s.weeklyTicker.Stop()
	}
	if s.maintenanceTicker != nil {
		s.maintenanceTicker.Stop()
	}
	close(s.stopChan)
}
