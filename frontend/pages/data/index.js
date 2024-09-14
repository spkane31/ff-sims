import React from "react";
import { Box, Paper, Typography } from "@mui/material";
import { DataGrid } from "@mui/x-data-grid";

// Create a functional component for the page
const Data = () => {
  const [perfectRosters, setPerfectRosters] = React.useState(null);
  const [pointsOnBench, setPointsOnBench] = React.useState(null);

  React.useEffect(() => {
    fetch("/api/perfectrosters")
      .then((res) => {
        return res.json();
      })
      .then((data) => {
        setPerfectRosters(
          data.map((roster) => {
            return { ...roster, id: Math.random() };
          })
        );
      });
  }, []);

  React.useEffect(() => {
    fetch("/api/pointsonbench")
      .then((res) => {
        return res.json();
      })
      .then((data) => {
        setPointsOnBench(
          data.map((roster) => {
            return { ...roster, id: Math.random() };
          })
        );
      });
  }, []);

  return (
    <Box
      sx={{
        maxWidth: "100%",
        overflowX: "auto",
        paddingTop: "25px",
      }}
    >
      <Paper
        sx={{
          minWidth: 750,
          minHeight: 400,
        }}
      >
        <PerfectRosters perfectRosters={perfectRosters} />
        <Box sx={{ marginTop: "25px" }} />
        <PointsOnBench pointsOnBench={pointsOnBench} />
        <Box sx={{ marginTop: "25px" }} />
      </Paper>
    </Box>
  );
};

const PerfectRosters = ({ perfectRosters }) => {
  if (perfectRosters === null) {
    return <div>Loading...</div>;
  }

  const columns = [
    { field: "owner", headerName: "Owner", flex: 1, minWidth: 130 },
    { field: "week", headerName: "Week", flex: 1, type: "number" },
    { field: "year", headerName: "Year", flex: 1, type: "number" },
  ];

  return (
    <>
      <Typography
        variant="h5"
        sx={{ textAlign: "center", marginBottom: "15px" }}
      >
        Perfect Rosters
      </Typography>
      <DataGrid
        columns={columns}
        rows={perfectRosters}
        rowHeight={30}
        autoHeight
        hideFooter
      />
    </>
  );
};
const PointsOnBench = ({ pointsOnBench }) => {
  if (pointsOnBench === null) {
    return <div>Loading...</div>;
  }

  const columns = [
    { field: "owner", headerName: "Owner", flex: 1, minWidth: 130 },
    {
      field: "missingPoints",
      headerName: "Points Left on Bench",
      flex: 1,
      type: "number",
    },
  ];

  return (
    <>
      <Typography
        variant="h5"
        sx={{ textAlign: "center", marginBottom: "15px" }}
      >
        Points Left on Bench
      </Typography>
      <DataGrid
        columns={columns}
        rows={pointsOnBench}
        rowHeight={30}
        autoHeight
        hideFooter
      />
    </>
  );
};

export default Data;
