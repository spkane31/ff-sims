import * as React from "react";
import Table from "@mui/material/Table";
import TableBody from "@mui/material/TableBody";
import TableCell from "@mui/material/TableCell";
import TableContainer from "@mui/material/TableContainer";
import TableHead from "@mui/material/TableHead";
import TableRow from "@mui/material/TableRow";
import Paper from "@mui/material/Paper";

// TOOD seankane: add an optional title to tables

export default function BasicTable({ columns, data, title }) {
  if (data === undefined) {
    return <></>;
  }
  console.log("data: ", data);
  return (
    <TableContainer component={Paper}>
      <Table sx={{ minWidth: 650 }} aria-label="simple table">
        <TableHead>
          <TableRow>
            {columns.map((item, idx) => {
              if (idx === 0) {
                return <TableCell key={item}>{item}</TableCell>;
              }
              return (
                <TableCell align="right" key={item}>
                  {item}
                </TableCell>
              );
            })}
          </TableRow>
        </TableHead>
        <TableBody>
          {data === undefined ? (
            <></>
          ) : (
            data.map((row) => (
              <TableRow
                key={row[0]}
                sx={{ "&:last-child td, &:last-child th": { border: 0 } }}
              >
                {row.map((cell, idx) => {
                  if (idx >= columns.length) {
                    return;
                  }
                  if (idx === 0) {
                    return (
                      <TableCell
                        component="th"
                        scope="row"
                        key={row[0] + "" + idx + "" + cell}
                      >
                        {cell}
                      </TableCell>
                    );
                  }
                  return (
                    <TableCell
                      align="right"
                      key={row[0] + "" + idx + "" + cell}
                    >
                      {cell}
                    </TableCell>
                  );
                })}
              </TableRow>
            ))
          )}
        </TableBody>
      </Table>
    </TableContainer>
  );
}
