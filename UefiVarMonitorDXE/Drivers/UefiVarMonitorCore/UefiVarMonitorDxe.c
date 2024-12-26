#include <Uefi.h>
#include <Guid/EventGroup.h>
#include <Library/DebugLib.h>
#include <Library/UefiBootServicesTableLib.h>
#include <Library/UefiRuntimeServicesTableLib.h>
#include <library/UefiRuntimeLib.h>

static EFI_EVENT g_SetVaMapEvent;
static EFI_GET_VARIABLE g_GetVariable;
static EFI_SET_VARIABLE g_SetVariable;

/**
 * @brief Handles GetVariable runtime service calls.
 */
static
EFI_STATUS
EFIAPI
HandleGetVariable (
    IN CHAR16* VariableName,
    IN EFI_GUID* VendorGuid,
    OUT UINT32* Attributes OPTIONAL,
    IN OUT UINTN* DataSize,
    OUT VOID* Data OPTIONAL
    )
{
    EFI_STATUS status;
    UINTN effectiveDataSize;

    //
    // Invoke the original GetVariable service, and log this service invocation.
    //
    status = g_GetVariable(VariableName, VendorGuid, Attributes, DataSize, Data);
    effectiveDataSize = EFI_ERROR(status) ? 0 : *DataSize;
    DebugPrint(DEBUG_VERBOSE,
               "G: %g Size=%08x %s: %r\n",
               VendorGuid,
               effectiveDataSize,
               VariableName,
               status);

    return status;
}

/**
 * @brief Handles SetVariable runtime service calls.
 */
static
EFI_STATUS
EFIAPI
HandleSetVariable (
    IN CHAR16* VariableName,
    IN EFI_GUID* VendorGuid,
    IN UINT32 Attributes,
    IN UINTN DataSize,
    IN VOID* Data
    )
{
    EFI_STATUS status;

    //
    // Invoke the original SetVariable service, and log this service invocation.
    //
    status = g_SetVariable(VariableName, VendorGuid, Attributes, DataSize, Data);
    DebugPrint(DEBUG_VERBOSE,
               "S: %g Size=%08x %s: %r\n",
               VendorGuid,
               DataSize,
               VariableName,
               status);

    return status;
}

/**
 * @brief Converts global pointers from physical-mode ones to virtual-mode ones.
 */
static
VOID
EFIAPI
HandleSetVirtualAddressMap (
    IN EFI_EVENT Event,
    IN VOID* Context
    )
{
    EFI_STATUS status;
    VOID* currentAddress;

    currentAddress = (VOID*)g_GetVariable;
    status = gRT->ConvertPointer(0, (VOID**)&g_GetVariable);
    ASSERT_EFI_ERROR(status);
    DEBUG((DEBUG_ERROR,
           "GetVariable relocated from %p to %p\n",
           currentAddress,
           g_GetVariable));

    currentAddress = (VOID*)g_SetVariable;
    status = gRT->ConvertPointer(0, (VOID**)&g_SetVariable);
    ASSERT_EFI_ERROR(status);
    DEBUG((DEBUG_ERROR,
           "SetVariable relocated from %p to %p\n",
           currentAddress,
           g_SetVariable));
}

/**
 * @brief Exchanges a pointer in the EFI System Table.
 */
static
EFI_STATUS
ExchangePointerInServiceTable (
    IN OUT VOID** AddressToUpdate,
    IN VOID* NewPointer,
    OUT VOID** OriginalPointer OPTIONAL
    )
{
    EFI_STATUS status;
    EFI_TPL tpl;

    ASSERT(*AddressToUpdate != NewPointer);

    //
    // Disable interrupt.
    //
    tpl = gBS->RaiseTPL(TPL_HIGH_LEVEL);

    //
    // Save the current value if needed and update the pointer.
    //
    if (OriginalPointer != NULL)
    {
        *OriginalPointer = *AddressToUpdate;
    }
    *AddressToUpdate = NewPointer;

    //
    // Update the CRC32 in the EFI System Table header.
    //
    gST->Hdr.CRC32 = 0;
    status = gBS->CalculateCrc32(&gST->Hdr, gST->Hdr.HeaderSize, &gST->Hdr.CRC32);
    ASSERT_EFI_ERROR(status);

    gBS->RestoreTPL(tpl);
    return status;
}

/**
 * @brief Cleans up changes made by this module and release resources.
 *
 * @details This function handles pertical clean up and is safe for multiple calls.
 */
static
VOID
Cleanup (
    VOID
    )
{
    EFI_STATUS status;

    ASSERT(EfiAtRuntime() == FALSE);

    if (gST->RuntimeServices->SetVariable == HandleSetVariable)
    {
        status = ExchangePointerInServiceTable(
                                    (VOID**)&gST->RuntimeServices->SetVariable,
                                    (VOID*)g_SetVariable,
                                    NULL);
        ASSERT_EFI_ERROR(status);
    }

    if (gST->RuntimeServices->GetVariable == HandleGetVariable)
    {
        status = ExchangePointerInServiceTable(
                                    (VOID**)&gST->RuntimeServices->GetVariable,
                                    (VOID*)g_GetVariable,
                                    NULL);
        ASSERT_EFI_ERROR(status);
    }

    if (g_SetVaMapEvent != NULL)
    {
        status = gBS->CloseEvent(&g_SetVaMapEvent);
        ASSERT_EFI_ERROR(status);
        g_SetVaMapEvent = NULL;
    }
}

/**
 * @brief The module entry point.
 */
EFI_STATUS
EFIAPI
UefiVarMonitorDxeInitialize (
    IN EFI_HANDLE ImageHandle,
    IN EFI_SYSTEM_TABLE* SystemTable
    )
{
    EFI_STATUS status;

    DEBUG((DEBUG_ERROR, "Driver being loaded\n"));

    //
    // Register a notification for SetVirtualAddressMap call.
    //
    status = gBS->CreateEventEx(EVT_NOTIFY_SIGNAL,
                                TPL_CALLBACK,
                                HandleSetVirtualAddressMap,
                                NULL,
                                &gEfiEventVirtualAddressChangeGuid,
                                &g_SetVaMapEvent);
    if (EFI_ERROR(status))
    {
        DEBUG((DEBUG_ERROR, "CreateEventEx failed : %r\n", status));
        goto Exit;
    }

    //
    // Install hooks.
    //
    status = ExchangePointerInServiceTable((VOID**)&gST->RuntimeServices->GetVariable,
                                           (VOID*)HandleGetVariable,
                                           (VOID**)&g_GetVariable);
    if (EFI_ERROR(status))
    {
        DEBUG((DEBUG_ERROR, "ExchangeTablePointer(GetVariable) failed : %r\n", status));
        goto Exit;
    }
    status = ExchangePointerInServiceTable((VOID**)&gST->RuntimeServices->SetVariable,
                                           (VOID*)HandleSetVariable,
                                           (VOID**)&g_SetVariable);
    if (EFI_ERROR(status))
    {
        DEBUG((DEBUG_ERROR, "ExchangeTablePointer(SetVariable) failed : %r\n", status));
        goto Exit;
    }

Exit:
    if (EFI_ERROR(status))
    {
        Cleanup();
    }
    return status;
}

/**
 * @brief Handles unload request of this module.
 */
EFI_STATUS
EFIAPI
UefiVarMonitorDxeUnload (
    IN EFI_HANDLE ImageHandle
    )
{
    Cleanup();
    return EFI_SUCCESS;
}
