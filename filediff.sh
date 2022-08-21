#!/bin/bash

THERE=$HOME/src/gabbi/gabbi/tests/gabbits_intercept
HERE=$HOME/src/gobbi/testdata

comm  -2 -3 <(ls $THERE) <(ls $HERE)
