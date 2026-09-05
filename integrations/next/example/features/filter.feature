Feature: Filter the board
  As someone tracking work
  I want to see only active or only done tasks
  So that I can focus

  Scenario: Only done tasks under the Done filter
    When I show only Done tasks
    Then "Ship to Vercel" is listed
    And "Write the sightmap" is not listed
    When I show only Active tasks
    Then "Write the sightmap" is listed
    And "Ship to Vercel" is not listed
