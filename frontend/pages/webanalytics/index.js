import React from "react";
import { DataGrid } from "@mui/x-data-grid";
import { Box, Paper, Typography, Button } from "@mui/material";
import { Line } from "react-chartjs-2";
import Chart from "chart.js/auto";

export default function Home() {
  const [requests, setRequests] = React.useState(null);
  const [grouped, setCountGrouped] = React.useState(null);

  React.useEffect(() => {
    fetch("/api/webanalytics")
      .then((res) => res.json())
      .then((data) => {
        setRequests(data);
      });
  }, []);

  // when requests is updated, group the number of requests by 1 hour increments
  React.useEffect(() => {
    if (requests === null) return;
    const grouped = requests.reduce((acc, req) => {
      if (req.isFronted !== undefined && !req.isFronted) {
        return;
      }
      const timestamp = new Date(req.timestamp);
      const date = timestamp.toISOString().split("T")[0];
      acc[date] = acc[date] ? acc[date] + 1 : 1;
      return acc;
    }, {});

    // convert the map to an array of objects
    const groupedArr = Object.keys(grouped).map((key) => ({
      id: key,
      hour: key,
      count: grouped[key],
    }));

    setCountGrouped(groupedArr);
  }, [requests]);

  return requests === null ? (
    <div>Loading...</div>
  ) : (
    <Box
      sx={{
        paddingTop: "20px",
      }}
    >
      <DataGrid
        rows={grouped}
        columns={[
          { field: "hour", headerName: "hour" },
          { field: "count", headerName: "count" },
        ]}
        autosizeOnMount
        hideFooter
      />
      <Box sx={{ marginTop: "20px" }} />
      <LineGraphOfHits data={grouped} />
      <Box sx={{ marginTop: "20px" }} />
      <DataGrid
        rows={requests}
        columns={[
          { field: "endpoint", headerName: "Endpoint", width: 200 },
          { field: "method", headerName: "Method", width: 150 },
          { field: "body", headerName: "Body", width: 200 },
          { field: "userAgent", headerName: "User Agent", width: 200 },
          { field: "isFrontend", headerName: "Is Frontend", width: 150 },
          { field: "timestamp", headerName: "Timestamp", width: 200 },
        ]}
      />
    </Box>
  );
}

function LineGraphOfHits({ data }) {
  if (data === null) {
    return null;
  }
  const labels = data.map((d) => d.hour);
  const counts = data.map((d) => d.count);
  const chartData = {
    labels: labels,
    datasets: [
      {
        label: "Number of hits",
        data: counts,
        fill: false,
        borderColor: "rgb(75, 192, 192)",
        tension: 0.1,
      },
    ],
  };
  return (
    <Paper>
      <Typography variant="h6">Number of hits by hour</Typography>
      <Line data={chartData} />
    </Paper>
  );
}
