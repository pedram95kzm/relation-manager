import { render, screen, cleanup } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { Preferences } from "../i18n";
import { ScoreTables } from "./Results";
import type { Scores } from "../types";
afterEach(cleanup);
it("keeps unknown dimensions visibly distinct from numeric zero in accessible score tables", () => {
  localStorage.setItem("relations-locale", "en");
  const score = {
    answered: 0,
    total: 3,
    unknown: 3,
    omitted: 0,
    completeness: 0,
    status: "insufficient_data",
    band: null,
  };
  const scores: Scores = {
    status: "available",
    respondents: {
      self: {
        answered: 2,
        total: 6,
        unknown: 3,
        omitted: 1,
        numeric_completeness: 1 / 3,
        dimensions: [
          {
            ...score,
            id: "missing",
            label: { en: "Missing perspective", fa: "" },
            score: null,
          },
          {
            ...score,
            id: "zero",
            label: { en: "Measured zero", fa: "" },
            score: 0,
            answered: 2,
            unknown: 0,
            omitted: 1,
            completeness: 2 / 3,
          },
        ],
      },
    },
  };
  render(
    <Preferences>
      <ScoreTables scores={scores} />
    </Preferences>,
  );
  expect(screen.getByRole("table")).toBeInTheDocument();
  expect(
    screen.getByRole("row", { name: /Missing perspective/ }),
  ).toHaveTextContent("—");
  expect(screen.getByRole("row", { name: /Measured zero/ })).toHaveTextContent(
    "0.0",
  );
  expect(screen.getByText("2/3 · 67%")).toBeInTheDocument();
});
