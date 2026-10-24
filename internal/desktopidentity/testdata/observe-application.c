#define _GNU_SOURCE
#include <gio/gio.h>
#include <dlfcn.h>
#include <stdio.h>
static void started(GApplication *app, void *unused) {
 fprintf(stderr, "IDENTITY registered=%d id=%s prgname=%s\n", g_application_get_is_registered(app), g_application_get_application_id(app), g_get_prgname());
}
int g_application_run(GApplication *app, int argc, char **argv) {
 fprintf(stderr, "IDENTITY before-run registered=%d id=%s prgname=%s\n", g_application_get_is_registered(app), g_application_get_application_id(app), g_get_prgname());
 g_signal_connect(app, "startup", G_CALLBACK(started), NULL);
 int (*run)(GApplication*, int, char**) = dlsym(RTLD_NEXT,"g_application_run");
 return run(app, argc, argv);
}
