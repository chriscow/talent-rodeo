I need you to review all of the code and identify opportunities to refactor that enable:

1. Clarity:  It should be very easy to read and follow.  Functions should be clearly named and well documented using Go standard library style documentation. Functions should be easy to reason about and not be complex.
2. Testability: Functions should be small enough to contain basic functionality that is easy to test.  This generally means ensuring functions do one thing.  It also means that we isolate code that we aren't going to test, such as calling third party APIs etc so that we can minimize mocking.  We can add comments to indicate they aren't tested since they are only third party calls.
3. Optimized: I think there are areas of optimization possible.  Many redundant calls to the database that, perhaps could either be write-through cached (see @firestore.go for examples) or other things.
4. Safe: Safe from both a security stand point but also good practices like nil pointer checks, argument validation etc.

analyze all the code carefully and identify areas of improvement.  Make a list and a plan to incrementally improve and test along the way to ensure we didn't break anything.  If there are opportunities to write tests before starting to ensure nothing breaks, that's probably a good thing too.