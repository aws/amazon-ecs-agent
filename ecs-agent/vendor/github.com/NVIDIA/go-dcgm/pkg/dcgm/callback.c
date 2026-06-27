#include <stdint.h>

#include "dcgm_structs.h"

int violationNotify(dcgmPolicyCallbackResponse_t *response, uint64_t userData) {
    int ViolationRegistration(dcgmPolicyCallbackResponse_t *, uint64_t);
    return ViolationRegistration(response, userData);
}
