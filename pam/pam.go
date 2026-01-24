package pam

/*
#include<security/pam_appl.h>
#include<stdio.h>
#include<stdlib.h>

void custom_print(char *s){
	printf("%s\n", s);
	free(s);
}
*/
import "C"

func PrintC(s string, kill chan bool) error{
	select{
	case <-kill:
		return nil
	default:
		C.custom_print(C.CString(s))
		return nil
	}
}
