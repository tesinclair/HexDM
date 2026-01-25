#include "pam_raw.h"

int p_conv(int num_msg, const struct pam_message **msg,
            struct pam_response **resp, void *appdata_ptr){
    struct pam_response *reply = NULL;
    struct credentials *creds = (struct credentials *)appdata_ptr;

    reply = calloc(num_msg, sizeof(struct pam_response));
    if (reply == NULL){
        return PAM_BUF_ERR;
    }

    for (int i = 0; i<num_msg; i++){
        reply[i].resp_retcode = 0;
        reply[i].resp = NULL;

        switch(msg[i]->msg_style){
            case PAM_PROMPT_ECHO_OFF:
                if (strncmp(creds->password, "", 1) == 0){
                    // fprintd mode
                    return PAM_CONV_ERR; // kill pam
                }
                reply[i].resp = strdup(creds->password);
                break;
            case PAM_PROMPT_ECHO_ON:
                reply[i].resp = strdup("");
                break;
            case PAM_ERROR_MSG:
            case PAM_TEXT_INFO:
                break;
        }
    }
    
    *resp = reply;
    return PAM_SUCCESS;
}

/*
void sanatize(char **in){
}

int probe_fprintd(char *user){
    char command[256];
    char buffer[1024];
    int enrolled = 0;

    sanatize(&user);

    snprintf(command, sizeof(command), "fprintd-list %s 2>&1", user);

    FILE *fp = popen(command, "r");
    if (fp == NULL){
        return FAILED_FPRINTD_PROBE;
    }

    while (fgets(buffer, sizeof(buffer), fp) != NULL){
        if (strstr(buffer, "fingers:")){
            enrolled = 1
        }
    }

    pclose(fp);

    return enrolled;
}*/

int authenticate(const char *user, const char *password){
    pam_handle_t *pamh = NULL;
    int retval;
    
    struct credentials creds;
    creds.username = user;
    creds.password = password;

    struct pam_conv conv = {
        p_conv,
        &creds
    };

    retval = pam_start("system-auth", creds.username, &conv, &pamh);
    if (retval != PAM_SUCCESS){
        return retval;
    }

    retval = pam_authenticate(pamh, 0);
    if (retval != PAM_SUCCESS){
        return retval;
    }
    retval = pam_acct_mgmt(pamh, 0);
    if (retval != PAM_SUCCESS){
        return retval;
    }

    pam_end(pamh, retval);
    if (retval == PAM_SUCCESS){
        return SUCCESS;
    }
    
    return retval;
}

const char *get_err_code(int errcode){
    return pam_strerror(NULL, errcode);
}
