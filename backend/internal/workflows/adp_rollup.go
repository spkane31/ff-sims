package workflows

import (
	"fmt"

	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/sdk/workflow"

	"backend/internal/activities"
	"backend/internal/models"
)

func ADPRollupDispatcher(ctx workflow.Context) (ADPRollupDispatchReport, error) {
	ara := &activities.ADPRollupActivities{}
	actCtx := workflow.WithActivityOptions(ctx, defaultActivityOptions)

	var seasons []string
	if err := workflow.ExecuteActivity(actCtx, ara.ListADPSeasons).Get(ctx, &seasons); err != nil {
		return ADPRollupDispatchReport{}, err
	}

	var report ADPRollupDispatchReport
	for _, season := range seasons {
		for _, seg := range models.AllADPSegments() {
			cwo := workflow.ChildWorkflowOptions{
				WorkflowID:        fmt.Sprintf("%s-%s", season, seg.Key()),
				TaskQueue:         TaskQueueADP,
				ParentClosePolicy: enumspb.PARENT_CLOSE_POLICY_ABANDON,
			}
			params := SegmentSeasonADPParams{Segment: seg, Season: season}
			f := workflow.ExecuteChildWorkflow(workflow.WithChildOptions(ctx, cwo), SegmentSeasonADPRollupWorkflow, params)
			if err := f.GetChildWorkflowExecution().Get(ctx, nil); err != nil {
				workflow.GetLogger(ctx).Warn("failed to start SegmentSeasonADPRollupWorkflow",
					"segment", seg.Key(), "season", season, "error", err)
				continue
			}
			report.SegmentsScheduled++
		}
	}
	return report, nil
}

// SegmentSeasonADPRollupWorkflow computes and upserts ADP for one
// (segment, season) pair. A compute failure is logged rather than returned,
// so one bad segment/season doesn't surface as a workflow failure.
func SegmentSeasonADPRollupWorkflow(ctx workflow.Context, params SegmentSeasonADPParams) (SegmentADPReport, error) {
	ara := &activities.ADPRollupActivities{}
	actCtx := workflow.WithActivityOptions(ctx, defaultActivityOptions)

	var res activities.ADPRollupResult
	if err := workflow.ExecuteActivity(actCtx, ara.ComputeSegmentSeasonADP, activities.ComputeSegmentSeasonADPParams{
		Segment: params.Segment,
		Season:  params.Season,
	}).Get(ctx, &res); err != nil {
		workflow.GetLogger(ctx).Warn("ComputeSegmentSeasonADP failed",
			"segment", params.Segment.Key(), "season", params.Season, "error", err)
		return SegmentADPReport{}, nil
	}
	return SegmentADPReport{PlayersUpserted: res.PlayersUpserted}, nil
}
