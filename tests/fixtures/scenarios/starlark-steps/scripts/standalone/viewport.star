#!/usr/bin/env atmos
# output="viewport" is accepted next to "stream" and "capture"; without a terminal it streams.
result = atmos.run(["version"], output = "viewport")
print("viewport accepted")
