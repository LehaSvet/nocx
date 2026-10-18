# P3 publisher measurement

This report records the fixed publish footprint measured by `TestMeasuredMaximumIsStill`
in `publisher_measure_test.go`. The test measures production bundle bytes written through
the filesystem seam and counts calls across publish paths.

## Current maximum

- Maximum filesystem-seam calls per publish: **63**.
- Maximum bytes written per publish: **87,061**.
- Measured worst path: replacement plus sweep, with the launch carrier absent.
- Fault position used for the reported worst case: `remove#4`.

The 2026-10-16 measurement is 518 bytes below the previous 86,535-byte maximum. The
reduction comes from removing the redundant one-time waiting-question text from the bash
and zsh shell wrappers (nocx-xn63t.4.2). The filesystem-call maximum did not change.

The later 2026-10-16 measurement is 980 bytes above that 86,017-byte figure. The growth
comes from the bash and zsh wrappers installing their Ctrl+C trap for the whole agent
enrolment instead of only the wait for a question, and from closing an interval a cancel
lands on after admission (nocx-xn63t.6.29). The filesystem-call maximum did not change.

The 2026-10-18 measurement is 64 bytes above that 86,997-byte figure, at 87,061. The growth
is the launch state a spawned worker's tool surface needs: the shell records the
`tool_surface` classification beside `local`, clears it with the rest of the launch state,
and stages the private MCP config on it. Both wrappers carry it, so both moved
(nocx-xn63t.6.49). The filesystem-call maximum did not change.

Keep these values in sync with `measuredMaxPublishCalls` and `measuredMaxPublishBytes` in
`publisher_measure_test.go`. Update this report and both constants whenever the measured
maximum changes.
