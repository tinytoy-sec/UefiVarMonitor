#ifndef __UEFI_VAR_MONITOR_EX_DXE_H__
#define __UEFI_VAR_MONITOR_EX_DXE_H__

#if defined(NTDDI_VERSION)

//
// For NT drivers.
//
#include <wdm.h>
#define EFIAPI      __cdecl
typedef WCHAR       CHAR16;
typedef CHAR        CHAR8;
typedef SIZE_T      EFI_STATUS;
typedef SIZE_T      UINTN;

#else

//
// For UEFI drivers.
//
#include <Uefi.h>

#endif

//
// {3DEC99FB-86B4-4EED-B4D8-4E6ADDE56F95}
//
CONST GUID g_BackdoorGuid =
{ 0x3dec99fb, 0x86b4, 0x4eed, { 0xb4, 0xd8, 0x4e, 0x6a, 0xdd, 0xe5, 0x6f, 0x95 } };

typedef enum _VARIABLE_CALLBACK_TYPE
{
    VariableCallbackGet,
    VariableCallbackSet,
} VARIABLE_CALLBACK_TYPE;

typedef enum _OPERATION_TYPE
{
    OperationPre,
    OperationPost,
} OPERATION_TYPE;

//
// The single log entry type in the log buffer.
//
#if defined(_MSC_VER)
#pragma warning(push)
#pragma warning(disable: 4200)
#endif
typedef struct _VARIABLE_LOG_ENTRY
{
    CHAR16 VariableName[64];
    GUID VendorGuid;
    VARIABLE_CALLBACK_TYPE CallbackType;
    UINT32 Attributes;
    EFI_STATUS Status;
    CHAR8 StatusMessage[32];
    UINTN DataSize;
    UINT8 Data[0];
} VARIABLE_LOG_ENTRY;
#if defined(_MSC_VER)
#pragma warning(pop)
#endif

//
// The parameter type of the Get/SetVariable service callback.
//
typedef struct _VARIABLE_CALLBACK_PARAMETERS
{
    VARIABLE_CALLBACK_TYPE CallbackType;
    OPERATION_TYPE OperationType;
    union
    {
        struct
        {
            CHAR16** VariableName;  // Mutable
            GUID** VendorGuid;      // Mutable
            UINT32** Attributes;    // Mutable; (*Attributes) may be NULL
            UINTN** DataSize;       // Mutable
            VOID** Data;            // Mutable; (*Data) may be NULL
            BOOLEAN Succeeded;      // Immutable
            CHAR8* StatusMessage;   // Immutable
        } Get;

        struct
        {
            CHAR16** VariableName;  // Mutable
            GUID** VendorGuid;      // Mutable
            UINT32* Attributes;     // Mutable
            UINTN* DataSize;        // Mutable
            VOID** Data;            // Mutable
            BOOLEAN Succeeded;      // Immutable
            CHAR8* StatusMessage;   // Immutable
        } Set;
    } Parameters;
} VARIABLE_CALLBACK_PARAMETERS;

//
// Get/SetVariable service callback type.
//
// The callbacks are allowed to modify values pointed from Parameters if it is
// indicated as mutable.
//
// Returning TRUE on Pre-callback prevent the service to be executed.
// On Post-callback, the return value is ignored.
//
typedef
BOOLEAN
(EFIAPI*VARIABLE_CALLBACK) (
    IN OUT VARIABLE_CALLBACK_PARAMETERS* Parameters
    );

#endif
