interface Window {
  /**
   * The report data.
   * Go injects it into the generated report.
   * sample.ts sets sample data during development and in the plain template.
   */
  __GIT_WHEN_DATA__?: import("./scripts/report/types").ReportData;
}
