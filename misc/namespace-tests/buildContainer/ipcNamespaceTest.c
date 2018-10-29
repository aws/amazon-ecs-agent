/*
This file is derived from the Kubernetes Project, Copyright 2016 The Kubernetes Authors.

The original code may be found at:
 https://github.com/kubernetes/kubernetes/blob/17b22cb01c647d50ed186c8a94f9c7aa2d9bcd2e/build/pause/pause.c

Copyright 2016 The Kubernetes Authors.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

#include <signal.h>
#include <stdio.h>
#include <stdlib.h>
#include <sys/types.h>
#include <sys/wait.h>
#include <unistd.h>
#include <sys/ipc.h>
#include <sys/sem.h>

int createdSemId;

static void sigdown(int signo) {
  psignal(signo, "Shutting down, got signal");
  semctl(createdSemId, 0, IPC_RMID);
  exit(0);
}

int main() {
  if (sigaction(SIGINT, &(struct sigaction){.sa_handler = sigdown}, NULL) < 0)
    return 1;
  if (sigaction(SIGTERM, &(struct sigaction){.sa_handler = sigdown}, NULL) < 0)
    return 2;

  key_t key = 5;
  int semflg = IPC_CREAT | 0644;
  int nsems = 1;

  if ((createdSemId = semget(key, nsems, semflg)) == -1) {
    perror("semget: semget failed. Unable to create IPC semaphore");
    exit(1);
  }
  for (;;) {
      sleep(1);
  }
  return 1;
}
