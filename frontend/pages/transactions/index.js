import React from "react";
import Box from "@mui/material/Box";
import Table from "@mui/material/Table";
import TableBody from "@mui/material/TableBody";
import TableCell from "@mui/material/TableCell";
import TableContainer from "@mui/material/TableContainer";
import TableHead from "@mui/material/TableHead";
import TableRow from "@mui/material/TableRow";
import Typography from "@mui/material/Typography";
import Paper from "@mui/material/Paper";
import { FormControl, Select, MenuItem } from "@mui/material";

const columns = [
  {
    field: "year",
    headerName: "Year",
    sortable: true,
  },
  {
    field: "player_name",
    headerName: "Player",
    sortable: true,
    width: 200,
  },
  {
    field: "player_position",
    headerName: "Position",
    sortable: true,
  },
  {
    field: "total_actual_points",
    headerName: "Points",
    type: "number",
    sortable: true,
  },
  {
    field: "total_projected_points",
    headerName: "Projected Points",
    type: "number",
    sortable: true,
  },
  {
    field: "diff",
    headerName: "Difference",
    type: "number",
    sortable: true,
  },
];

// Create a functional component for the page
const Data = () => {
  const [transactions, setTransactions] = React.useState(null);
  const [selected, setSelected] = React.useState([]);

  const OPTIONS = ["FA ADDED", "TRADED", "WAIVER ADDED", "DROPPED"];

  React.useEffect(() => {
    fetch("/api/transactions")
      .then((res) => res.json())
      .then((data) => {
        data = data
          .map((item, index) => ({ ...item, id: index + 1 }))
          .sort((a, b) => b.date - a.date);
        setTransactions(data);
      });
  }, []);

  if (transactions === null) {
    return <div>Loading...</div>;
  }

  return (
    <Box sx={{ flexGrow: 1, padding: "15px" }}>
      <Typography variant="h4" align="center" sx={{ padding: "15px 0" }}>
        Transactions
      </Typography>
      <FormControl fullWidth>
        <Select value={selected} onChange={handleChange}>
          {OPTIONS.map((year) => (
            <MenuItem key={year} value={year}>
              {year}
            </MenuItem>
          ))}
        </Select>
      </FormControl>
      <TableContainer component={Paper}>
        <Table aira-labelledby="tableTitle" size="small">
          <TableHead>
            <TableRow>
              <TableCell>Date</TableCell>
              <TableCell>Owner</TableCell>
              <TableCell>Transaction</TableCell>
              <TableCell>Player</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {transactions.map((row, index) => {
              const extraRows = row.transactions
                .slice(1)
                .map((transaction, idx) => (
                  <TableRow key={`${index}-${idx}`}>
                    <TableCell></TableCell>
                    <TableCell>{transaction.owner}</TableCell>
                    <TableCell>{transaction.transaction_type}</TableCell>
                    <TableCell>{transaction.player_name}</TableCell>
                  </TableRow>
                ));
              return (
                <>
                  <TableRow key={index}>
                    <TableCell>{row.date}</TableCell>
                    <TableCell>{row.transactions[0].owner}</TableCell>
                    <TableCell>
                      {row.transactions[0].transaction_type}
                    </TableCell>
                    <TableCell>{row.transactions[0].player_name}</TableCell>
                  </TableRow>
                  {extraRows}
                </>
              );
            })}
          </TableBody>
        </Table>
      </TableContainer>
    </Box>
  );
};

export default Data;
