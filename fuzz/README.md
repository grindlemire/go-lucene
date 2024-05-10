# What is this package?

This package contains all the necessary code to fuzz test go-lucene. However it requires a few imports
to do so and uses pg_query to validate the produced queries. Moving it to this directory allows the top level
mod file to remain clean of dependencies while still allowing for the fuzz testing.