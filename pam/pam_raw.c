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
                    reply[i].resp = NULL; // move on
                }else{
                    reply[i].resp = strdup(creds->password);
                }
                break;
            case PAM_PROMPT_ECHO_ON:
                reply[i].resp = strdup("");
                break;
            case PAM_ERROR_MSG:
                fprintf(stderr, "PAM_ERROR_MSG: %s\n", msg[i]->msg);
                break;
            case PAM_TEXT_INFO:
                fprintf(stderr, "PAM_TEXT_INFO: %s\n", msg[i]->msg);
                break;
        }
    }
    
    *resp = reply;
    return PAM_SUCCESS;
}

int authenticate(pam_handle_t **pamh, const char *user, const char *password, const char *tty){
    int ret;
    
    struct credentials creds;
    creds.username = user;
    creds.password = password;

    struct pam_conv conv = {
        p_conv,
        &creds
    };

    if (strncmp(creds.password, "", 1) == 0){
        // if fprintd is available
        ret = pam_start("hexdm", creds.username, &conv, pamh);
    }else{
        ret = pam_start("hexdm_nofprint", creds.username, &conv, pamh);
    }

    if (ret != PAM_SUCCESS){
        pam_end(*pamh, ret);
        return ret;
    }

    if (tty != NULL){
        pam_set_item(*pamh, PAM_TTY, strdup(tty));
    }

    ret = pam_set_item(*pamh, PAM_USER, creds.username);
    if (ret != PAM_SUCCESS){
        pam_end(*pamh, ret);
        return ret;
    }

    ret = pam_authenticate(*pamh, 0);
    if (ret != PAM_SUCCESS){
        pam_end(*pamh, ret);
        return ret;
    }
    ret = pam_acct_mgmt(*pamh, 0);
    if (ret != PAM_SUCCESS){
        pam_end(*pamh, ret);
        return ret;
    }

    struct passwd *pw = getpwnam(creds.username);
    if (!pw){
        pam_end(*pamh, PAM_ABORT);
        return -1;
    }
    uid_t uid = pw->pw_uid;

    char runtime_env[64];
    snprintf(runtime_env, sizeof(runtime_env),
            "XDG_RUNTIME_DIR=/run/user/%d", uid);

    pam_putenv(*pamh, runtime_env);

    ret = pam_setcred(*pamh, PAM_ESTABLISH_CRED);
    if (ret != PAM_SUCCESS){
        pam_end(*pamh, ret);
        return ret;
    }

    ret = pam_open_session(*pamh, 0);
    if (ret != PAM_SUCCESS){
        pam_end(*pamh, ret);
        return ret;
    }

    return ret;
}

int teardown(pam_handle_t *pamh){
    if (pamh == NULL){
        return PAM_SUCCESS;
    }

    // Undo authenticate() in reverse order, and always carry on to
    // pam_end: an error in one step must not leave the session open or
    // the handle allocated.
    int ret = pam_close_session(pamh, 0);
    int cred_ret = pam_setcred(pamh, PAM_DELETE_CRED);
    if (ret == PAM_SUCCESS){
        ret = cred_ret;
    }
    pam_end(pamh, ret);

    return ret;
}

const char *get_err_code(int errcode){
    return pam_strerror(NULL, errcode);
}
