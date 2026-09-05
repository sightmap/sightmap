Feature: Triage from the detail page
  As someone tracking work
  I want to finish a task from its own page
  So that the board reflects it when I come back

  Scenario: Complete a task from its detail page
    When I open "Write the sightmap"
    Then the task page shows it as "Active"
    When I mark it done
    Then the task page shows it as "Done"
    When I go back to the board
    And I show only Done tasks
    Then "Write the sightmap" is listed as done
