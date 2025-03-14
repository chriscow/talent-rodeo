Please review @SEARCH.md .  I believe have not determined our strategy for implementing "Search Term Construction" and "Search Execution".  More precisely, when "Ready to Search" in @profile-preferences.html is clicked, a single message is sent via NATS to the @search_scheduler.go .  This message needs to be expanded into 1. the actual search keywords we will use to perform a search. I want to figure out if we may need to perform multiple searches to capture adaquate coverage of the skills and 2. Each of the above "searches" will get multiple messages published to cover the necessary page ranges for the subscriber tier.  3. Also the job scoring system seems to have been lost in the @SEARCH.md doc:

+10 points for each "Must Have" skill match
+5 points for each "Prefer" skill match
+1 point for each "No Preference" skill match
-3 points for each "Avoid" skill that appears
Immediate exclusion for "Must Avoid" matches


My concern about the actual search keywords and how many search it will take are explained in this scenario:

Let's say a user has "Go" as "Must Have" and "Python" as "No Preference".  We could safely do one keyword search for "Go" (or "Golang") and all results can be analyzed afterward by an LLM.  If the job description (JD) has Go as the main language (not a nice to have) then it doesn't really matter if the JD contains Python since it is "No Preference".  The job weighting will not get a bump up or down if it contains Python.

In the same scenario, if the user had "Python" as "Prefer" we could run a single search with "Go Python" as keywords.  This is treated as an "OR" query but does not prioritize "Go" and may not even include "Go" so we should probably just search for "Go" as the only keyword and we will subsequently weight the job higher if "Python" is also a requiremnet.  If both "Go" and "Python" are "Must Have" we could use them both as keywords in a single search and filter ones that don't have both as primary with an LLM.

Anyway, this is what we need to work out and document our strategy in @SEARCH.md 