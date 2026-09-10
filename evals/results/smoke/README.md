# Scripted browser smoke measurements

These are observation and fixture checks, not agent effectiveness results. Model tokens are null throughout. Wall time includes observations and scripted actions, excluding setup/reset and final assertions.

| Task | Mode | Trial | Assertion | Actions | Observation bytes | Capture ms | Wall ms |
|---|---|---:|---|---:|---:|---:|---:|
| save-contact | raw-ax | 1 | true | 2 | 49209 | 13.138 | 15.211 |
| save-contact | sightmap | 1 | true | 2 | 1922 | 15.277 | 16.330 |
| save-contact | sightkick | 1 | not run | — | — | — | — |
| save-contact | stagehand-facade | 1 | not run | — | — | — | — |
| filter-select | raw-ax | 1 | true | 2 | 47096 | 2.944 | 3.845 |
| filter-select | sightmap | 1 | true | 2 | 1786 | 13.244 | 13.958 |
| filter-select | sightkick | 1 | not run | — | — | — | — |
| filter-select | stagehand-facade | 1 | not run | — | — | — | — |
| open-report | raw-ax | 1 | true | 1 | 24431 | 1.803 | 24.819 |
| open-report | sightmap | 1 | true | 1 | 955 | 8.556 | 29.854 |
| open-report | sightkick | 1 | not run | — | — | — | — |
| open-report | stagehand-facade | 1 | not run | — | — | — | — |
| save-contact | sightmap | 2 | true | 2 | 1922 | 15.048 | 16.617 |
| save-contact | raw-ax | 2 | true | 2 | 49618 | 8.548 | 10.077 |
| save-contact | sightkick | 2 | not run | — | — | — | — |
| save-contact | stagehand-facade | 2 | not run | — | — | — | — |
| filter-select | sightmap | 2 | true | 2 | 1786 | 14.657 | 15.445 |
| filter-select | raw-ax | 2 | true | 2 | 47096 | 4.592 | 5.400 |
| filter-select | sightkick | 2 | not run | — | — | — | — |
| filter-select | stagehand-facade | 2 | not run | — | — | — | — |
| open-report | sightmap | 2 | true | 1 | 955 | 6.230 | 28.791 |
| open-report | raw-ax | 2 | true | 1 | 24431 | 1.468 | 22.365 |
| open-report | sightkick | 2 | not run | — | — | — | — |
| open-report | stagehand-facade | 2 | not run | — | — | — | — |
| save-contact | raw-ax | 3 | true | 2 | 49618 | 2.971 | 3.831 |
| save-contact | sightmap | 3 | true | 2 | 1922 | 12.461 | 18.533 |
| save-contact | sightkick | 3 | not run | — | — | — | — |
| save-contact | stagehand-facade | 3 | not run | — | — | — | — |
| filter-select | raw-ax | 3 | true | 2 | 47096 | 2.836 | 3.529 |
| filter-select | sightmap | 3 | true | 2 | 1786 | 18.972 | 19.719 |
| filter-select | sightkick | 3 | not run | — | — | — | — |
| filter-select | stagehand-facade | 3 | not run | — | — | — | — |
| open-report | raw-ax | 3 | true | 1 | 24431 | 6.262 | 28.116 |
| open-report | sightmap | 3 | true | 1 | 955 | 7.137 | 29.645 |
| open-report | sightkick | 3 | not run | — | — | — | — |
| open-report | stagehand-facade | 3 | not run | — | — | — | — |
