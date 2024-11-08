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
import {
  FormControl,
  FormGroup,
  Checkbox,
  FormControlLabel,
} from "@mui/material";

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
  const [options, setOptions] = React.useState(["TRADED"]);

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

  const handlePositionChange = (event) => {
    if (event.target.checked) {
      setOptions([...options, event.target.value]);
    } else {
      setOptions(options.filter((pos) => pos !== event.target.value));
    }
  };

  if (transactions === null) {
    return <div>Loading...</div>;
  }

  const filteredTransactions = transactions.filter((transaction) =>
    options.includes(transaction.transactions[0].transaction_type)
  );

  return (
    <Box sx={{ flexGrow: 1, padding: "15px" }}>
      <Typography variant="h4" align="center" sx={{ padding: "15px 0" }}>
        Transactions
      </Typography>
      <Box sx={{ padding: "10px" }} align="center">
        <FormControl component="fieldset">
          <FormGroup aria-label="year" row>
            {["FA ADDED", "TRADED", "WAIVER ADDED", "DROPPED"].map((option) => (
              <FormControlLabel
                key={option}
                control={
                  <Checkbox
                    checked={options.includes(option)}
                    onChange={handlePositionChange}
                    value={option}
                  />
                }
                label={option}
                labelPlacement="bottom"
              />
            ))}
          </FormGroup>
        </FormControl>
      </Box>
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
            {filteredTransactions.map((row, index) => {
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
