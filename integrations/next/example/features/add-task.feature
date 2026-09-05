Feature: Add a task
  As someone tracking work
  I want to add a task by title
  So that it shows up on the board

  Scenario: A new task appears on the board, not yet done
    When I add a task "Ship the proof of concept"
    Then the board lists "Ship the proof of concept"
    And it is not done
