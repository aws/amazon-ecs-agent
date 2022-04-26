
### 1. Open ppt, explain services demoed, auth, db reviews

### 2. Paste below command in black, explain each param

`./start_sc_task.sh --network awsvpc --service auth --port 80 --other-port 8081 --dependencies none --sc-ingress 0`

### 3. Open ecs console, show containers and explain contents, env vars, images, etc

### 4. paste below in black, note sc-ingress (non-default), explain we'll come back to this

`./start_sc_task.sh --network awsvpc --service db --port 3306 --other-port 3307 --dependencies none --sc-ingress 15000`

### 5. paste below in blue, note ephemeral ports for ingress and egress

`curl -s 10.0.0.113:9901/listeners`

### 6. paste below command in blue, explain what it is

`watch "curl -s 10.0.0.113:9901/stats | grep 'http_proxy.downstream_rq_2xx'"`

### 7. paste below in blue, note counter increase and envoy server header

`curl -v 10.0.0.113:80`

### 8. paste below in blue, note no envoy header or counter increase since this is non-SC

`curl -v 10.0.0.113:8081`

### 9. paste below in green

`watch "curl -s 10.0.0.113:9901/stats | grep 'http_proxy.downstream_rq_2xx'"`

### 10. paste below in green, note no envoy header, and no counter increase (non default)

`curl -v 10.0.0.113:3306`

### 11. paste below in green, note envoy header, counter increase (non-default)

`curl -v 10.0.0.113:15000`

### 12. paste below in black, explain dependencies

`./start_sc_task.sh --network awsvpc --service reviews --port 8080 --other-port 9090 --dependencies auth:10.0.0.90:80,db:10.0.0.114:15000 --sc-ingress 0`

### 13. paste below in orange, note counter cascade
`watch "curl -s 10.0.0.113:9901/stats | grep 'http_proxy.downstream_rq_2xx'"`

`curl -v 10.0.0.113:8080`

`docker exec -it 123 curl http://auth:80`

`docker exec -it 123 curl http://db:15000`

### 14. If there's time, paste below in black

`./start_sc_task.sh --network awsvpc --service products --port 9080 --other-port 9081 --dependencies reviews:10.0.0.140:8080 --sc-ingress 0`

`curl -v 10.0.0.113:9080`