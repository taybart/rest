/PASS/ { printf("\033[32m" $0 "\033[0m\n") }
/FAIL/ { printf("\033[31m" $0 "\033[0m\n") }
/RUN/ { printf("\033[33m" $0 "\033[0m\n") }
!/PASS|FAIL|RUN/ { print($0) }
