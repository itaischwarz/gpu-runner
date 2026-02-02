# Complex Job Examples for Testing GPU Runner

This document provides examples of complex jobs you can use to test and stress-test your GPU job runner.

## Table of Contents
1. [Data Processing Jobs](#data-processing-jobs)
2. [Machine Learning Simulation Jobs](#machine-learning-simulation-jobs)
3. [Concurrent Processing Jobs](#concurrent-processing-jobs)
4. [Resource-Intensive Jobs](#resource-intensive-jobs)
5. [Error Handling & Retry Jobs](#error-handling--retry-jobs)
6. [Multi-Step Pipeline Jobs](#multi-step-pipeline-jobs)
7. [Long-Running Jobs](#long-running-jobs)

---

## Data Processing Jobs

### 1. Log File Analysis
```bash
# Create sample log files and analyze patterns
gpucli submit --cmd "for i in {1..1000}; do echo \"[$(date)] Log entry \$i\" >> app.log; done && grep -c 'Log entry' app.log && wc -l app.log" --storage 10485760

# Process CSV data
gpucli submit --cmd "echo 'id,name,value' > data.csv && for i in {1..500}; do echo \"\$i,item_\$i,\$((RANDOM % 1000))\" >> data.csv; done && awk -F',' '{sum+=\$3} END {print \"Total:\",sum}' data.csv" --storage 10485760
```

### 2. JSON Data Transformation
```bash
gpucli submit --cmd "echo '{\"records\":[]}' > output.json && for i in {1..100}; do echo '{\"id\":''\$i'',\"timestamp\":\"'$(date -Iseconds)'\",\"value\":''\$((RANDOM % 100))''}' >> temp.json; done && cat temp.json" --storage 10485760
```

### 3. Text Processing & Word Count
```bash
gpucli submit --cmd "for i in {1..50}; do fortune 2>/dev/null || echo 'Lorem ipsum dolor sit amet consectetur adipiscing elit' >> text.txt; done && wc -w text.txt && sort text.txt | uniq -c | sort -rn | head -10" --storage 10485760
```

---

## Machine Learning Simulation Jobs

### 4. Matrix Operations Simulation
```bash
gpucli submit --cmd "python3 -c \"import random; matrix = [[random.random() for _ in range(100)] for _ in range(100)]; result = sum(sum(row) for row in matrix); print(f'Matrix sum: {result}')\"" --storage 10485760
```

### 5. Numerical Computing Simulation
```bash
gpucli submit --cmd "python3 -c \"
import math
import time

def monte_carlo_pi(n=1000000):
    inside = 0
    for _ in range(n):
        x = random.random()
        y = random.random()
        if x*x + y*y <= 1:
            inside += 1
    return 4 * inside / n

import random
start = time.time()
pi_estimate = monte_carlo_pi()
elapsed = time.time() - start
print(f'Pi estimate: {pi_estimate}, Time: {elapsed}s')
\"" --storage 10485760
```

### 6. Data Generation & Statistics
```bash
gpucli submit --cmd "python3 -c \"
import random
import statistics

data = [random.gauss(100, 15) for _ in range(10000)]
print(f'Mean: {statistics.mean(data):.2f}')
print(f'Median: {statistics.median(data):.2f}')
print(f'StdDev: {statistics.stdev(data):.2f}')
print(f'Min: {min(data):.2f}, Max: {max(data):.2f}')
\"" --storage 10485760
```

---

## Concurrent Processing Jobs

### 7. Parallel File Processing
```bash
gpucli submit --cmd "for i in {1..10}; do (echo 'Processing file '\$i && sleep 0.1 && echo 'Done '\$i) & done; wait && echo 'All files processed'" --storage 10485760
```

### 8. Multi-Process Data Pipeline
```bash
gpucli submit --cmd "mkfifo pipe1 pipe2 && (seq 1 1000 > pipe1 &) && (cat pipe1 | grep '[05]$' > pipe2 &) && cat pipe2 | wc -l && rm pipe1 pipe2" --storage 10485760
```

---

## Resource-Intensive Jobs

### 9. CPU-Intensive Fibonacci Calculation
```bash
gpucli submit --cmd "python3 -c \"
def fib(n):
    if n <= 1: return n
    return fib(n-1) + fib(n-2)

for i in range(20, 30):
    print(f'fib({i}) = {fib(i)}')
\"" --storage 10485760 --maxRetries 1
```

### 10. Compression & Decompression
```bash
gpucli submit --cmd "dd if=/dev/urandom bs=1M count=5 2>/dev/null | base64 > random.txt && gzip -9 random.txt && ls -lh random.txt.gz && gunzip random.txt.gz && echo 'Decompressed successfully'" --storage 25165824
```

### 11. Hash Computation
```bash
gpucli submit --cmd "for i in {1..1000}; do echo 'data'\$i | sha256sum; done | head -20 && echo '... (truncated)'" --storage 10485760
```

---

## Error Handling & Retry Jobs

### 12. Randomly Failing Job (Tests Retry Logic)
```bash
gpucli submit --cmd "if [ \$((RANDOM % 3)) -eq 0 ]; then echo 'Success!'; exit 0; else echo 'Failed, retrying...'; exit 1; fi" --storage 10485760 --maxRetries 5
```

### 13. Timeout Stress Test
```bash
# This will timeout if WORKER_JOB_TIMEOUT is 30s (default)
gpucli submit --cmd "echo 'Starting long task...' && sleep 35 && echo 'This should not print'" --storage 10485760
```

### 14. Gradual Success Job
```bash
gpucli submit --cmd "
ATTEMPT=\$(date +%s | tail -c 2)
if [ \$ATTEMPT -gt 80 ]; then
    echo 'Success after multiple attempts'
    exit 0
else
    echo 'Attempt failed, will retry'
    exit 1
fi
" --storage 10485760 --maxRetries 3
```

---

## Multi-Step Pipeline Jobs

### 15. ETL Pipeline Simulation
```bash
gpucli submit --cmd "
echo '=== Extract Phase ===' &&
seq 1 100 > raw_data.txt &&
echo 'Extracted 100 records' &&

echo '=== Transform Phase ===' &&
awk '{print \$1*2}' raw_data.txt > transformed.txt &&
echo 'Transformed data' &&

echo '=== Load Phase ===' &&
cat transformed.txt | awk '{sum+=\$1; count++} END {print \"Loaded\",count,\"records, Sum:\",sum}' &&

echo '=== Pipeline Complete ==='
" --storage 10485760
```

### 16. Build & Test Simulation
```bash
gpucli submit --cmd "
echo '>>> Step 1: Setup' &&
mkdir -p src build test &&
echo 'print(\"Hello World\")' > src/main.py &&

echo '>>> Step 2: Build' &&
cp src/main.py build/app.py &&

echo '>>> Step 3: Test' &&
python3 build/app.py > test/output.txt &&

echo '>>> Step 4: Verify' &&
if grep -q 'Hello World' test/output.txt; then
    echo '✓ All tests passed'
    exit 0
else
    echo '✗ Tests failed'
    exit 1
fi
" --storage 10485760
```

### 17. Data Validation Pipeline
```bash
gpucli submit --cmd "
echo 'id,email,age' > users.csv &&
echo '1,user1@test.com,25' >> users.csv &&
echo '2,invalid-email,30' >> users.csv &&
echo '3,user3@test.com,invalid' >> users.csv &&
echo '4,user4@test.com,35' >> users.csv &&

echo '=== Validation Report ===' &&
total=\$(wc -l < users.csv) &&
valid_emails=\$(grep -cE '[a-zA-Z0-9]+@[a-zA-Z0-9]+\.[a-zA-Z]+' users.csv || echo 0) &&
echo \"Total rows: \$total\" &&
echo \"Valid emails: \$valid_emails\" &&
echo \"Validation complete\"
" --storage 10485760
```

---

## Long-Running Jobs

### 18. Progressive Task with Status Updates
```bash
gpucli submit --cmd "
for i in {1..10}; do
    echo \"[Progress: \$((i*10))%] Processing batch \$i/10\"
    sleep 2
    dd if=/dev/zero of=batch_\$i.dat bs=1M count=1 2>/dev/null
done
echo 'Task completed successfully'
ls -lh batch_*.dat
" --storage 25165824
```

### 19. Incremental Data Processing
```bash
gpucli submit --cmd "
echo 'Starting incremental processing...' &&
total=0 &&
for i in {1..20}; do
    value=\$((RANDOM % 100))
    total=\$((total + value))
    echo \"Iteration \$i: value=\$value, running_total=\$total\"
    sleep 1
done &&
echo \"Final total: \$total\"
" --storage 10485760
```

---

## Storage Tier Testing

### 20. Small Storage Tier (10MB)
```bash
gpucli submit --cmd "dd if=/dev/urandom bs=1M count=8 2>/dev/null | base64 > data.txt && ls -lh data.txt && echo 'Fits in 10MB tier'" --storage 10485760
```

### 21. Medium Storage Tier (25MB)
```bash
gpucli submit --cmd "dd if=/dev/urandom bs=1M count=20 2>/dev/null | base64 > data.txt && ls -lh data.txt && echo 'Fits in 25MB tier'" --storage 26214400
```

### 22. Large Storage Tier (50MB)
```bash
gpucli submit --cmd "dd if=/dev/urandom bs=1M count=40 2>/dev/null | base64 > data.txt && ls -lh data.txt && echo 'Fits in 50MB tier'" --storage 52428800
```

---

## Advanced Testing Scenarios

### 23. Job Cancellation Test
```bash
# Submit a long job, then cancel it
JOB_ID=$(gpucli submit --cmd "sleep 100 && echo 'This should not complete'" --storage 10485760 | grep -oP 'ID: \K\d+')
sleep 2
gpucli cancel --id $JOB_ID --reason "Testing cancellation"
```

### 24. Concurrent Job Submission
```bash
# Submit multiple jobs at once to test worker pool
for i in {1..10}; do
    gpucli submit --cmd "echo 'Job \$i starting' && sleep 5 && echo 'Job \$i done'" --storage 10485760 &
done
wait
```

### 25. Job Queue Saturation Test
```bash
# Submit more jobs than worker capacity to test queuing
for i in {1..20}; do
    gpucli submit --cmd "echo 'Queued job '\$i && sleep 2" --storage 10485760
done
```

---

## Monitoring & Debugging Jobs

### 26. System Resource Check
```bash
gpucli submit --cmd "echo '=== System Info ===' && uname -a && echo '=== Disk Usage ===' && df -h . && echo '=== Memory Info ===' && free -h 2>/dev/null || echo 'free command not available' && echo '=== Process Info ===' && ps aux | head -10" --storage 10485760
```

### 27. Environment Variable Inspection
```bash
gpucli submit --cmd "env | sort | grep -E '(PATH|USER|HOME|PWD)'" --storage 10485760
```

### 28. Network Connectivity Test (if tools available)
```bash
gpucli submit --cmd "ping -c 3 8.8.8.8 2>/dev/null || echo 'Ping not available'; echo 'DNS test:'; nslookup google.com 2>/dev/null || echo 'nslookup not available'" --storage 10485760
```

---

## Tips for Testing

1. **Start Simple**: Begin with basic jobs and gradually increase complexity
2. **Monitor Resources**: Watch worker logs and Redis queue length during testing
3. **Test Failure Cases**: Use jobs that intentionally fail to verify retry logic
4. **Concurrent Load**: Submit multiple jobs simultaneously to test worker pool
5. **Storage Limits**: Test jobs near storage tier boundaries
6. **Timeout Behavior**: Submit jobs that exceed `WORKER_JOB_TIMEOUT` to verify cancellation
7. **Log Inspection**: Use `gpucli status <job-id>` to verify logging works correctly

## Environment Variable Testing

Remember to test different configurations:

```bash
# Test with more workers
WORKER_COUNT=5 docker-compose up

# Test with longer timeout
WORKER_JOB_TIMEOUT=60s docker-compose up

# Test with debug logging
LOG_LEVEL=debug docker-compose up
```
